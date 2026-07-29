package hub

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type PublicEvent struct {
	AgentName string           `json:"agent_name"`
	RunPath   []string         `json:"run_path,omitempty"`
	Output    *PublicOutput    `json:"output,omitempty"`
	Action    *adk.AgentAction `json:"action,omitempty"`
	Error     string           `json:"error,omitempty"`
}

type PublicOutput struct {
	Message *schema.Message `json:"message,omitempty"`
	Custom  any             `json:"custom,omitempty"`
}

func ToPublicEvent(ev *adk.AgentEvent) PublicEvent {
	out := PublicEvent{
		AgentName: ev.AgentName,
		Action:    ev.Action,
	}
	if ev.Err != nil {
		out.Error = ev.Err.Error()
	}
	if len(ev.RunPath) > 0 {
		out.RunPath = make([]string, 0, len(ev.RunPath))
		for _, step := range ev.RunPath {
			name := step.String()
			if name != "" {
				out.RunPath = append(out.RunPath, name)
			}
		}
	}
	if ev.Output != nil {
		msg, _, err := adk.GetMessage(ev)
		if err == nil && msg != nil {
			out.Output = &PublicOutput{Message: msg}
		} else if ev.Output.CustomizedOutput != nil {
			out.Output = &PublicOutput{Custom: ev.Output.CustomizedOutput}
		}
	}
	return out
}

// --- OpenAI Chat Completions compatible types ---

type OpenAIChatCompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []OpenAIChatChoice `json:"choices"`
	Usage   *OpenAIUsage       `json:"usage,omitempty"`
}

type OpenAIChatChoice struct {
	Index        int               `json:"index"`
	Message      OpenAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type OpenAIChatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall  `json:"tool_calls,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatCompletionChunk struct {
	ID      string                  `json:"id"`
	Object  string                  `json:"object"`
	Created int64                   `json:"created"`
	Model   string                  `json:"model"`
	Choices []OpenAIChatChunkChoice `json:"choices"`
}

type OpenAIChatChunkChoice struct {
	Index        int             `json:"index"`
	Delta        OpenAIChatDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type OpenAIChatDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall  `json:"tool_calls,omitempty"`
}

// RunResultToOpenAI converts a RunResult into an OpenAI Chat Completion response.
func RunResultToOpenAI(result *RunResult, model string) *OpenAIChatCompletionResponse {
	finishReason := "stop"
	if len(result.Interrupts) > 0 {
		finishReason = "tool_calls"
	}

	msg := OpenAIChatMessage{Role: "assistant", Content: result.Output}

	// Extract tool_calls from the last assistant message that has them.
	for i := len(result.Events) - 1; i >= 0; i-- {
		ev := result.Events[i]
		if ev.Output == nil || ev.Output.Message == nil {
			continue
		}
		m := ev.Output.Message
		if m.Role != schema.Assistant || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: OpenAIToolFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
		break
	}

	// Gather usage from the last event with ResponseMeta.
	var usage *OpenAIUsage
	for i := len(result.Events) - 1; i >= 0; i-- {
		ev := result.Events[i]
		if ev.Output == nil || ev.Output.Message == nil {
			continue
		}
		if meta := ev.Output.Message.ResponseMeta; meta != nil && meta.Usage != nil {
			usage = &OpenAIUsage{
				PromptTokens:     meta.Usage.PromptTokens,
				CompletionTokens: meta.Usage.CompletionTokens,
				TotalTokens:      meta.Usage.PromptTokens + meta.Usage.CompletionTokens,
			}
			break
		}
	}

	return &OpenAIChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", result.RunID),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChatChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}

// EventToOpenAIChunk converts a PublicEvent into an OpenAI streaming chunk.
// Returns nil if the event should be skipped (e.g. tool messages).
func EventToOpenAIChunk(ev PublicEvent, runID string, model string, isFirst bool) *OpenAIChatCompletionChunk {
	if ev.Output == nil || ev.Output.Message == nil {
		return nil
	}
	msg := ev.Output.Message
	if msg.Role != schema.Assistant {
		return nil
	}

	delta := OpenAIChatDelta{}
	if isFirst {
		delta.Role = "assistant"
	}
	if msg.Content != "" {
		delta.Content = msg.Content
	}
	for _, tc := range msg.ToolCalls {
		delta.ToolCalls = append(delta.ToolCalls, OpenAIToolCall{
			ID:   tc.ID,
			Type: "function",
			Function: OpenAIToolFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	// Skip if delta is completely empty (no content, no tool calls, not first).
	if delta.Role == "" && delta.Content == "" && len(delta.ToolCalls) == 0 {
		return nil
	}

	return &OpenAIChatCompletionChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", runID),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChatChunkChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: nil,
		}},
	}
}

// OpenAIStopChunk returns the final chunk with finish_reason="stop".
func OpenAIStopChunk(runID string, model string, hasInterrupts bool) *OpenAIChatCompletionChunk {
	reason := "stop"
	if hasInterrupts {
		reason = "tool_calls"
	}
	return &OpenAIChatCompletionChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", runID),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChatChunkChoice{{
			Index:        0,
			Delta:        OpenAIChatDelta{},
			FinishReason: &reason,
		}},
	}
}

// MarshalSSEData marshals v to JSON and wraps it as an SSE data line.
func MarshalSSEData(v any) []byte {
	data, _ := json.Marshal(v)
	buf := make([]byte, 0, 6+len(data)+2)
	buf = append(buf, "data: "...)
	buf = append(buf, data...)
	buf = append(buf, '\n', '\n')
	return buf
}
