package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"knsight-go/internal/hub/skills"
)

// TestContextBuilder_NonSceneFiltersSceneSkills ensures the default
// (non-scene) deployment does NOT include scene/* skills in the supervisor
// prompt. This is the bug we hit in prod: scene/socsci-interference skills
// leaked into hub.prod.yaml's supervisor because BuildSystemPrompt used
// the unfiltered Loader methods.
func TestContextBuilder_NonSceneFiltersSceneSkills(t *testing.T) {
	tmpDir := t.TempDir()
	must := func(path, body string) {
		full := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	shared := "---\nname: shared-tool\ndescription: shared\nalways: true\n---\nshared body\n"
	scene := "---\nname: scene-only\ndescription: scene-only\nalways: true\n---\nscene body\n"
	must("_common/shared-tool/SKILL.md", shared)
	must("scene/socsci-interference/scene-only/SKILL.md", scene)

	loader := skills.NewLoader(tmpDir)
	if err := loader.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := len(loader.ListSkills()); got != 2 {
		t.Fatalf("want 2 loaded skills, got %d", got)
	}

	// Default (non-scene) ContextBuilder: must include shared, must NOT
	// include scene-only.
	cb := NewContextBuilder(nil, loader, "", "")
	out := cb.BuildSystemPrompt(AgentConfig{Name: "TestSup", Description: "x"}, nil)
	if !strings.Contains(out, "严禁调用 `todo_comment`") {
		t.Fatal("system prompt must explain that todo progress is not a tool")
	}
	if !strings.Contains(out, "shared-tool") {
		t.Fatalf("default prompt should include shared skill, got:\n%s", out)
	}
	if strings.Contains(out, "scene-only") {
		t.Fatalf("default prompt must NOT include scene/* skill, got:\n%s", out)
	}

	// Matching-scene ContextBuilder: must include both shared and scene.
	cb2 := NewContextBuilder(nil, loader, "", "socsci-interference")
	out2 := cb2.BuildSystemPrompt(AgentConfig{Name: "TestSup", Description: "x"}, nil)
	if !strings.Contains(out2, "shared-tool") {
		t.Fatalf("scene prompt should include shared skill")
	}
	if !strings.Contains(out2, "scene-only") {
		t.Fatalf("scene prompt should include matching scene skill")
	}

	// Different-scene ContextBuilder: must include shared but NOT
	// socsci-interference's scene skill.
	cb3 := NewContextBuilder(nil, loader, "", "other-scene")
	out3 := cb3.BuildSystemPrompt(AgentConfig{Name: "TestSup", Description: "x"}, nil)
	if !strings.Contains(out3, "shared-tool") {
		t.Fatalf("other-scene prompt should include shared skill")
	}
	if strings.Contains(out3, "scene-only") {
		t.Fatalf("other-scene prompt must NOT include socsci-interference skill")
	}
}
