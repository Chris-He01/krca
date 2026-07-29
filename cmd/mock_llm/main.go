package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []toolSpec    `json:"tools"`
}

type toolSpec struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Arguments   string `json:"arguments,omitempty"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

func main() {
	addr := flag.String("addr", ":8090", "mock llm listen address")
	reply := flag.String("reply", "xxx", "default reply content")
	transferTarget := flag.String("transfer-target", "InspectAgent", "agent name for transfer tool calls")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		lastUser := lastUserContent(req.Messages)
		hasToolResponse := hasToolRole(req.Messages)
		toolNames := toolNameSet(req.Tools)

		var resp chatResponse
		resp.ID = "chatcmpl-mock"
		resp.Object = "chat.completion"
		resp.Created = time.Now().Unix()
		resp.Model = req.Model

		if !hasToolResponse {
			if call := decideToolCall(lastUser, toolNames, *transferTarget); call != nil {
				resp.Choices = []chatChoice{{
					Index:        0,
					Message:      chatMessage{Role: "assistant", Content: "", ToolCalls: []toolCall{*call}},
					FinishReason: "tool_calls",
				}}
				writeJSON(w, resp)
				return
			}
		}

		content := *reply
		if lastUser != "" {
			content = content + " | echo: " + lastUser
		}
		resp.Choices = []chatChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}}
		writeJSON(w, resp)
	})

	log.Printf("mock llm listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("mock llm server error: %v", err)
	}
}

func lastUserContent(messages []chatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func hasToolRole(messages []chatMessage) bool {
	for _, msg := range messages {
		if msg.Role == "tool" {
			return true
		}
	}
	return false
}

func toolNameSet(tools []toolSpec) map[string]bool {
	set := make(map[string]bool)
	for _, tool := range tools {
		if tool.Function.Name != "" {
			set[tool.Function.Name] = true
		}
	}
	return set
}

func decideToolCall(input string, toolNames map[string]bool, transferTarget string) *toolCall {
	lower := strings.ToLower(input)
	if strings.Contains(lower, "approve") || strings.Contains(lower, "审批") {
		if toolNames["request_approval"] {
			return &toolCall{
				ID:   "toolcall-approval",
				Type: "function",
				Function: toolFunction{
					Name:      "request_approval",
					Arguments: `{"reason":"requires manual approval"}`,
				},
			}
		}
	}
	if toolNames["server_metrics"] {
		return &toolCall{
			ID:   "toolcall-metrics",
			Type: "function",
			Function: toolFunction{
				Name:      "server_metrics",
				Arguments: `{"metric":"memory","window":"5m"}`,
			},
		}
	}
	if toolNames["transfer_to_agent"] {
		return &toolCall{
			ID:   "toolcall-transfer",
			Type: "function",
			Function: toolFunction{
				Name:      "transfer_to_agent",
				Arguments: `{"agent_name":"` + transferTarget + `"}`,
			},
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
