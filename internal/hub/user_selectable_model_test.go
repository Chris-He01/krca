package hub

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUserSelectableModelMaxTokens(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want int
	}{
		{
			name: "plural",
			yaml: "label: Qwen\nmodel_id: Qwen/Qwen3.6-27B\nmax_tokens: 32768\n",
			want: 32768,
		},
		{
			name: "singular alias",
			yaml: "label: Qwen\nmodel_id: Qwen/Qwen3.6-27B\nmax_token: 16384\n",
			want: 16384,
		},
		{
			name: "plural takes precedence",
			yaml: "label: Qwen\nmodel_id: Qwen/Qwen3.6-27B\nmax_tokens: 32768\nmax_token: 16384\n",
			want: 32768,
		},
		{
			name: "empty value is omitted",
			yaml: "label: Qwen\nmodel_id: Qwen/Qwen3.6-27B\nmax_tokens:\n",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model UserSelectableModel
			if err := yaml.Unmarshal([]byte(tt.yaml), &model); err != nil {
				t.Fatalf("unmarshal model: %v", err)
			}
			if model.MaxTokens != tt.want {
				t.Fatalf("MaxTokens = %d, want %d", model.MaxTokens, tt.want)
			}
		})
	}
}

func TestBuildLLMConfigForModelOverridesMaxTokens(t *testing.T) {
	h := &Hub{
		cfg: Config{
			LLM: LLMConfig{
				BaseURL:   "http://default/v1",
				Model:     "default",
				APIKey:    "default-key",
				MaxTokens: 4096,
			},
		},
	}

	got := h.buildLLMConfigForModel(UserSelectableModel{
		ModelID:   "Qwen/Qwen3.6-27B",
		BaseURL:   "http://qwen/v1",
		MaxTokens: 32768,
	})
	if got.Model != "Qwen/Qwen3.6-27B" {
		t.Fatalf("Model = %q", got.Model)
	}
	if got.BaseURL != "http://qwen/v1" {
		t.Fatalf("BaseURL = %q", got.BaseURL)
	}
	if got.MaxTokens != 32768 {
		t.Fatalf("MaxTokens = %d, want 32768", got.MaxTokens)
	}
}

func TestBuildLLMConfigForModelOmitsUnconfiguredMaxTokens(t *testing.T) {
	h := &Hub{cfg: Config{LLM: LLMConfig{MaxTokens: 4096}}}

	got := h.buildLLMConfigForModel(UserSelectableModel{ModelID: "Qwen"})
	if got.MaxTokens != 0 {
		t.Fatalf("MaxTokens = %d, want 0", got.MaxTokens)
	}
}
