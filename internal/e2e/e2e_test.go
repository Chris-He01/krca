package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"knsight-go/internal/hub"
)

type chatResponse struct {
	RunID  string `json:"run_id"`
	Output string `json:"output"`
}

func TestEndToEndChat(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "e2e-ok",
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(llm.Close)

	mcpServer := server.NewMCPServer("mock", mcp.LATEST_PROTOCOL_VERSION)
	mcpServer.AddTool(mcp.NewTool("server_metrics", mcp.WithString("metric")), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	mcpTestServer := server.NewTestServer(mcpServer)
	t.Cleanup(mcpTestServer.Close)

	cfg := hub.Config{
		LLM: hub.LLMConfig{
			BaseURL: llm.URL + "/v1",
			Model:   "mock",
			APIKey:  "mock",
		},
		Tools: hub.ToolsConfig{
			MCPs: []hub.MCPConfig{
				{
					Name:   "testMcp",
					SSEURL: mcpTestServer.URL + "/sse",
				},
			},
		},
		Supervisor: hub.AgentConfig{
			Name:        "Supervisor",
			Description: "supervisor",
			Instruction: "delegate to sub agents",
		},
		SubAgents: []hub.AgentConfig{
			{
				Name:        "InspectAgent",
				Description: "inspect",
				Instruction: "use tools",
			},
		},
	}

	h, err := hub.NewHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewHub error: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Message string `json:"message"`
			RunID   string `json:"run_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if payload.RunID == "" {
			payload.RunID = "run-e2e"
		}
		result, err := h.Run(r.Context(), payload.RunID, payload.Message, "", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	reqBody, _ := json.Marshal(map[string]any{"message": "check metrics"})
	resp, err := http.Post(ts.URL+"/v1/chat", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if parsed.Output == "" {
		t.Fatalf("expected output, got empty")
	}
}
