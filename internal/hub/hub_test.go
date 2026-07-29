package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type llmRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

type llmResponse struct {
	Choices []struct {
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func TestHubInterruptResume(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llmRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		hasTool := false
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				hasTool = true
				break
			}
		}
		resp := llmResponse{}
		if !hasTool {
			resp.Choices = append(resp.Choices, struct {
				Message struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				Message: struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				}{
					Role:    "assistant",
					Content: "",
					ToolCalls: []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					}{
						{
							ID:   "approval-1",
							Type: "function",
							Function: struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							}{
								Name:      "request_approval",
								Arguments: `{"reason":"test"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			})
		} else {
			resp.Choices = append(resp.Choices, struct {
				Message struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				Message: struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				}{
					Role:    "assistant",
					Content: "approved and complete",
				},
				FinishReason: "stop",
			})
		}
		writeJSON(w, resp)
	}))
	t.Cleanup(llm.Close)

	cfg := Config{
		LLM: LLMConfig{
			BaseURL: llm.URL + "/v1",
			Model:   "mock",
			APIKey:  "mock",
		},
		Sandbox: SandboxConfig{
			AutoApprove: func() *bool { v := false; return &v }(),
		},
		Supervisor: AgentConfig{
			Name:        "Supervisor",
			Description: "supervisor",
			Instruction: "call request_approval",
		},
	}

	h, err := NewHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewHub error: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	runID := "run-interrupt"
	result, err := h.Run(context.Background(), runID, "need approval", "", nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result.Interrupts) == 0 {
		t.Fatalf("expected interrupt, got none")
	}

	targetID := result.Interrupts[0].ID
	resumed, err := h.Resume(context.Background(), runID, map[string]any{targetID: "ok"})
	if err != nil {
		t.Fatalf("Resume error: %v", err)
	}
	if resumed.Output == "" {
		t.Fatalf("expected output after resume")
	}
}

func TestHubMCPEnabled(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := llmResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			Message: struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls,omitempty"`
			}{
				Role:    "assistant",
				Content: "ok",
			},
			FinishReason: "stop",
		})
		writeJSON(w, resp)
	}))
	t.Cleanup(llm.Close)

	mcpServer := server.NewMCPServer("mock", mcp.LATEST_PROTOCOL_VERSION)
	mcpServer.AddTool(mcp.NewTool("server_metrics", mcp.WithString("metric")), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	mcpTestServer := server.NewTestServer(mcpServer)
	t.Cleanup(mcpTestServer.Close)

	cfg := Config{
		LLM: LLMConfig{
			BaseURL: llm.URL + "/v1",
			Model:   "mock",
			APIKey:  "mock",
		},
		Tools: ToolsConfig{
			MCPs: []MCPConfig{
				{
					Name:        "testMcp",
					Description: "test mcp",
					SSEURL:      mcpTestServer.URL + "/sse",
				},
			},
		},
		Supervisor: AgentConfig{
			Name:        "Supervisor",
			Description: "supervisor",
			Instruction: "use tools",
		},
		SubAgents: []AgentConfig{
			{
				Name:        "InspectAgent",
				Description: "inspect",
				Instruction: "use server_metrics tool",
			},
		},
	}

	h, err := NewHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewHub error: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	result, err := h.Run(context.Background(), "run-mcp", "get metrics", "", nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Output == "" {
		t.Fatalf("expected output, got empty")
	}
}

type stubInvokableTool struct {
	called bool
}

func (s *stubInvokableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "stub", Desc: "stub tool"}, nil
}

func (s *stubInvokableTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	s.called = true
	return "inner executed", nil
}

func TestApprovalToolAutoApprove(t *testing.T) {
	tool := NewApprovalTool()
	ctx := WithAutoApprove(context.Background(), true)
	out, err := tool.InvokableRun(ctx, `{"reason":"test"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if out != "approval auto-approved: test" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestApprovalWrapperAutoApproveBypassesInterrupt(t *testing.T) {
	inner := &stubInvokableTool{}
	wrapper := NewApprovalWrapper(inner)
	ctx := WithAutoApprove(context.Background(), true)
	out, err := wrapper.InvokableRun(ctx, `{"key":"value"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if out != "inner executed" {
		t.Fatalf("unexpected output: %q", out)
	}
	if !inner.called {
		t.Fatalf("expected inner tool to be called")
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}
