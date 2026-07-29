package hub

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
)

type ContextLengthError struct {
	InputTokens  int
	ContextLimit int
	RawMessage   string
}

type ContextCompactionEvent struct {
	Status         string `json:"status"`
	Model          string `json:"model,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	InputTokens    int    `json:"input_tokens,omitempty"`
	ContextLimit   int    `json:"context_limit,omitempty"`
	MessagesBefore int    `json:"messages_before,omitempty"`
	MessagesAfter  int    `json:"messages_after,omitempty"`
	BytesBefore    int    `json:"bytes_before,omitempty"`
	BytesAfter     int    `json:"bytes_after,omitempty"`
	Message        string `json:"message"`
}

type contextCompactionCallback func(ContextCompactionEvent)
type contextCompactionContextKey struct{}

func WithContextCompactionCallback(ctx context.Context, callback func(ContextCompactionEvent)) context.Context {
	if callback == nil {
		return ctx
	}
	return context.WithValue(ctx, contextCompactionContextKey{}, contextCompactionCallback(callback))
}

func emitContextCompaction(ctx context.Context, event ContextCompactionEvent) {
	callback, _ := ctx.Value(contextCompactionContextKey{}).(contextCompactionCallback)
	if callback != nil {
		callback(event)
	}
}

var contextLengthTokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)input\s*\(\s*(\d+)\s*tokens?\s*\).*context length\s*\(\s*(\d+)\s*tokens?\s*\)`),
	regexp.MustCompile(`(?i)maximum context length is\s*(\d+).*requested\s*(\d+)`),
	regexp.MustCompile(`(?i)requested\s*(\d+).*maximum context length is\s*(\d+)`),
	regexp.MustCompile(`(?i)input length\s*(\d+).*exceeds.*(?:limit|context length)\s*(\d+)`),
}

func detectContextLengthError(err error) (*ContextLengthError, bool) {
	if err == nil {
		return nil, false
	}
	texts := []string{err.Error(), describeErr(err)}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		texts = append(texts, apiErr.Message, fmt.Sprint(apiErr.Code), apiErr.Type)
	}
	raw := strings.Join(texts, "\n")
	lower := strings.ToLower(raw)
	triggers := []string{
		"context_length_exceeded",
		"longer than the model's context length",
		"maximum context length",
		"prompt is too long",
		"too many tokens",
	}
	matched := false
	for _, trigger := range triggers {
		if strings.Contains(lower, trigger) {
			matched = true
			break
		}
	}
	if !matched && strings.Contains(lower, "input length") && strings.Contains(lower, "exceeds") {
		matched = true
	}
	if !matched {
		return nil, false
	}

	info := &ContextLengthError{RawMessage: err.Error()}
	for i, pattern := range contextLengthTokenPatterns {
		match := pattern.FindStringSubmatch(raw)
		if len(match) != 3 {
			continue
		}
		first, _ := strconv.Atoi(match[1])
		second, _ := strconv.Atoi(match[2])
		if i == 1 {
			info.ContextLimit, info.InputTokens = first, second
		} else {
			info.InputTokens, info.ContextLimit = first, second
		}
		break
	}
	return info, true
}

type compactMessageBlock struct {
	messages  []*schema.Message
	protected bool
}

type ContextCompactionStats struct {
	MessagesBefore int
	MessagesAfter  int
	BytesBefore    int
	BytesAfter     int
}

func compactContextMessages(messages []*schema.Message, overflow *ContextLengthError, configuredLimit int, targetRatio float64, level int) ([]*schema.Message, ContextCompactionStats) {
	messages = sanitizeDanglingToolCalls(messages)
	stats := ContextCompactionStats{
		MessagesBefore: len(messages),
		BytesBefore:    messagesApproxBytes(messages),
	}
	if len(messages) == 0 {
		return messages, stats
	}

	blocks := buildCompactMessageBlocks(messages)
	lastUserBlock := -1
	for i := len(blocks) - 1; i >= 0; i-- {
		for _, msg := range blocks[i].messages {
			if msg != nil && msg.Role == schema.User {
				lastUserBlock = i
				break
			}
		}
		if lastUserBlock >= 0 {
			break
		}
	}
	for i := range blocks {
		if i == 0 || i == lastUserBlock || i >= len(blocks)-3 {
			blocks[i].protected = true
		}
	}

	toolMaxRunes := 2400
	textMaxRunes := 3200
	latestUserMaxRunes := 8000
	if level > 1 {
		toolMaxRunes = 900
		textMaxRunes = 1800
		latestUserMaxRunes = 4000
	}
	for i := range blocks {
		for j, msg := range blocks[i].messages {
			if msg == nil {
				continue
			}
			cloned := *msg
			if i < len(blocks)-2 {
				cloned.ReasoningContent = ""
			}
			if msg.Role == schema.Tool && utf8.RuneCountInString(msg.Content) > toolMaxRunes {
				cloned.Content = compactText(msg.Content, toolMaxRunes, "[older tool result compacted]")
			} else if msg.Role == schema.User && i == lastUserBlock &&
				utf8.RuneCountInString(msg.Content) > latestUserMaxRunes {
				cloned.Content = compactText(msg.Content, latestUserMaxRunes, "[latest delegated input compacted]")
			} else if msg.Role != schema.System && i != lastUserBlock && utf8.RuneCountInString(msg.Content) > textMaxRunes {
				cloned.Content = compactText(msg.Content, textMaxRunes, "[older message compacted]")
			}
			blocks[i].messages[j] = &cloned
		}
	}

	currentBytes := blocksApproxBytes(blocks)
	targetBytes := compactionTargetBytes(stats.BytesBefore, overflow, configuredLimit, targetRatio)
	var folded []string
	for i := 0; i < len(blocks) && currentBytes > targetBytes; i++ {
		if blocks[i].protected || len(blocks[i].messages) == 0 {
			continue
		}
		folded = append(folded, summarizeBlock(blocks[i]))
		currentBytes -= messagesApproxBytes(blocks[i].messages)
		blocks[i].messages = nil
	}

	out := make([]*schema.Message, 0, len(messages))
	for _, block := range blocks {
		if len(block.messages) == 0 {
			continue
		}
		out = append(out, block.messages...)
	}
	if len(folded) > 0 {
		summary := schema.UserMessage("[Earlier execution history compacted]\n" + strings.Join(folded, "\n"))
		insertAt := 0
		for insertAt < len(out) && out[insertAt] != nil && out[insertAt].Role == schema.System {
			insertAt++
		}
		out = append(out, nil)
		copy(out[insertAt+1:], out[insertAt:])
		out[insertAt] = summary
	}

	out = sanitizeDanglingToolCalls(out)
	stats.MessagesAfter = len(out)
	stats.BytesAfter = messagesApproxBytes(out)
	return out, stats
}

func buildCompactMessageBlocks(messages []*schema.Message) []compactMessageBlock {
	blocks := make([]compactMessageBlock, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		block := compactMessageBlock{messages: []*schema.Message{msg}}
		if msg != nil && msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			for i+1 < len(messages) && messages[i+1] != nil && messages[i+1].Role == schema.Tool {
				i++
				block.messages = append(block.messages, messages[i])
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func compactionTargetBytes(current int, overflow *ContextLengthError, configuredLimit int, targetRatio float64) int {
	if targetRatio <= 0 || targetRatio >= 1 {
		targetRatio = defaultContextCompactTarget
	}
	limit := configuredLimit
	input := 0
	if overflow != nil {
		if overflow.ContextLimit > 0 {
			limit = overflow.ContextLimit
		}
		input = overflow.InputTokens
	}
	if limit > 0 && input > 0 {
		target := int(float64(current) * (float64(limit) * targetRatio / float64(input)))
		if target > 0 && target < current {
			return target
		}
	}
	if limit > 0 {
		return int(float64(limit) * targetRatio * 3.5)
	}
	return int(float64(current) * 0.75)
}

func compactText(value string, maxRunes int, label string) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	head := maxRunes * 2 / 3
	tail := maxRunes - head
	return fmt.Sprintf("%s\n%s\n... %d runes omitted ...\n%s",
		label, string(runes[:head]), len(runes)-maxRunes, string(runes[len(runes)-tail:]))
}

func summarizeBlock(block compactMessageBlock) string {
	var parts []string
	for _, msg := range block.messages {
		if msg == nil {
			continue
		}
		switch {
		case msg.Role == schema.Assistant && len(msg.ToolCalls) > 0:
			var names []string
			for _, call := range msg.ToolCalls {
				if call.Function.Name != "" {
					names = append(names, call.Function.Name)
				}
			}
			parts = append(parts, "tool calls: "+strings.Join(names, ", "))
		case msg.Role == schema.Tool:
			name := msg.ToolName
			if name == "" {
				name = msg.ToolCallID
			}
			parts = append(parts, fmt.Sprintf("tool result %s (%d bytes)", name, len(msg.Content)))
		case msg.Content != "":
			parts = append(parts, fmt.Sprintf("%s message (%d bytes)", msg.Role, len(msg.Content)))
		}
	}
	if len(parts) == 0 {
		return "- older empty message block removed"
	}
	return "- " + strings.Join(parts, "; ")
}

func messagesApproxBytes(messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		if msg != nil {
			total += messageApproxBytes(msg)
		}
	}
	return total
}

func blocksApproxBytes(blocks []compactMessageBlock) int {
	total := 0
	for _, block := range blocks {
		total += messagesApproxBytes(block.messages)
	}
	return total
}
