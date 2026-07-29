package hub

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

const (
	llmTraceRequestTailMessages = 8
	llmTraceMessageMaxRunes     = 4000
	llmTraceReasoningMaxRunes   = 2000
	llmTraceToolArgsMaxRunes    = 2000
	llmTraceErrorMaxRunes       = 2000
)

// CompactLLMRoundRecord is the Redis-friendly representation of an LLM round.
// It keeps enough context for diagnosis without storing the growing prompt and
// full tool schemas again on every round.
type CompactLLMRoundRecord struct {
	SessionID           string                   `json:"session_id,omitempty"`
	RunID               string                   `json:"run_id"`
	UserID              string                   `json:"user_id,omitempty"`
	AgentName           string                   `json:"agent_name"`
	Round               int64                    `json:"round"`
	StartedAt           string                   `json:"started_at"`
	DurationNS          int64                    `json:"duration_ns"`
	Streaming           bool                     `json:"streaming"`
	RequestMessageCount int                      `json:"request_message_count"`
	RequestBytes        int                      `json:"request_bytes"`
	RequestTail         []CompactLLMTraceMessage `json:"request_tail,omitempty"`
	ToolNames           []string                 `json:"tool_names,omitempty"`
	Response            *CompactLLMTraceMessage  `json:"response,omitempty"`
	Error               string                   `json:"error,omitempty"`
}

type CompactLLMTraceMessage struct {
	Role               schema.RoleType      `json:"role"`
	Name               string               `json:"name,omitempty"`
	Content            string               `json:"content,omitempty"`
	ContentBytes       int                  `json:"content_bytes,omitempty"`
	ContentTruncated   bool                 `json:"content_truncated,omitempty"`
	Reasoning          string               `json:"reasoning,omitempty"`
	ReasoningBytes     int                  `json:"reasoning_bytes,omitempty"`
	ReasoningTruncated bool                 `json:"reasoning_truncated,omitempty"`
	ToolCallID         string               `json:"tool_call_id,omitempty"`
	ToolName           string               `json:"tool_name,omitempty"`
	ToolCalls          []CompactLLMToolCall `json:"tool_calls,omitempty"`
	PromptTokens       int                  `json:"prompt_tokens,omitempty"`
	CompletionTokens   int                  `json:"completion_tokens,omitempty"`
	TotalTokens        int                  `json:"total_tokens,omitempty"`
}

type CompactLLMToolCall struct {
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name"`
	Arguments          string `json:"arguments,omitempty"`
	ArgumentsBytes     int    `json:"arguments_bytes,omitempty"`
	ArgumentsTruncated bool   `json:"arguments_truncated,omitempty"`
}

func CompactLLMRound(record LLMRoundRecord) CompactLLMRoundRecord {
	requestBytes := 0
	for _, message := range record.Request {
		if message != nil {
			requestBytes += messageApproxBytes(message)
		}
	}

	start := len(record.Request) - llmTraceRequestTailMessages
	if start < 0 {
		start = 0
	}
	requestTail := make([]CompactLLMTraceMessage, 0, len(record.Request)-start)
	for _, message := range record.Request[start:] {
		if message != nil {
			requestTail = append(requestTail, compactLLMTraceMessage(message))
		}
	}

	toolNames := make([]string, 0, len(record.Tools))
	for _, tool := range record.Tools {
		if tool != nil && tool.Name != "" {
			toolNames = append(toolNames, tool.Name)
		}
	}

	var response *CompactLLMTraceMessage
	if record.Response != nil {
		compact := compactLLMTraceMessage(record.Response)
		response = &compact
	}

	return CompactLLMRoundRecord{
		SessionID:           record.SessionID,
		RunID:               record.RunID,
		UserID:              record.UserID,
		AgentName:           record.AgentName,
		Round:               record.Round,
		StartedAt:           record.StartedAt.Format("2006-01-02T15:04:05.000Z07:00"),
		DurationNS:          int64(record.Duration),
		Streaming:           record.Streaming,
		RequestMessageCount: len(record.Request),
		RequestBytes:        requestBytes,
		RequestTail:         requestTail,
		ToolNames:           toolNames,
		Response:            response,
		Error:               truncateTraceText(record.Error, llmTraceErrorMaxRunes),
	}
}

func compactLLMTraceMessage(message *schema.Message) CompactLLMTraceMessage {
	content, contentTruncated := truncateTraceTextWithFlag(message.Content, llmTraceMessageMaxRunes)
	reasoning, reasoningTruncated := truncateTraceTextWithFlag(message.ReasoningContent, llmTraceReasoningMaxRunes)
	compact := CompactLLMTraceMessage{
		Role:               message.Role,
		Name:               message.Name,
		Content:            content,
		ContentBytes:       len(message.Content),
		ContentTruncated:   contentTruncated,
		Reasoning:          reasoning,
		ReasoningBytes:     len(message.ReasoningContent),
		ReasoningTruncated: reasoningTruncated,
		ToolCallID:         message.ToolCallID,
		ToolName:           message.ToolName,
	}
	for _, call := range message.ToolCalls {
		args, truncated := truncateTraceTextWithFlag(call.Function.Arguments, llmTraceToolArgsMaxRunes)
		compact.ToolCalls = append(compact.ToolCalls, CompactLLMToolCall{
			ID:                 call.ID,
			Name:               call.Function.Name,
			Arguments:          args,
			ArgumentsBytes:     len(call.Function.Arguments),
			ArgumentsTruncated: truncated,
		})
	}
	if message.ResponseMeta != nil && message.ResponseMeta.Usage != nil {
		compact.PromptTokens = message.ResponseMeta.Usage.PromptTokens
		compact.CompletionTokens = message.ResponseMeta.Usage.CompletionTokens
		compact.TotalTokens = message.ResponseMeta.Usage.TotalTokens
	}
	return compact
}

func messageApproxBytes(message *schema.Message) int {
	data, err := json.Marshal(message)
	if err != nil {
		return len(message.Content) + len(message.ReasoningContent)
	}
	return len(data)
}

func truncateTraceText(value string, maxRunes int) string {
	truncated, _ := truncateTraceTextWithFlag(value, maxRunes)
	return truncated
}

func truncateTraceTextWithFlag(value string, maxRunes int) (string, bool) {
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value, false
	}
	runes := []rune(value)
	return string(runes[:maxRunes]) + "...", true
}
