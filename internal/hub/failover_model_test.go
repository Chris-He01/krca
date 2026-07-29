package hub

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestSanitizeDanglingToolCallsPreservesValidBlocks(t *testing.T) {
	messages := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "lookup",
				Arguments: "{}",
			},
		}}),
		schema.ToolMessage("ok", "call-1", schema.WithToolName("lookup")),
		schema.UserMessage("continue"),
	}

	got := sanitizeDanglingToolCalls(messages)
	if len(got) != len(messages) {
		t.Fatalf("expected valid block to be preserved, got %d messages", len(got))
	}
	if len(got[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool call to remain")
	}
	if got[2].Role != schema.Tool || got[2].ToolCallID != "call-1" {
		t.Fatalf("expected tool result to remain structured, got role=%s id=%q", got[2].Role, got[2].ToolCallID)
	}
}

// With the synthesize-and-keep repair, an orphan assistant tool_use that
// has a non-empty ID is no longer dropped — a placeholder tool_result is
// appended so the conversation stays well-formed for Anthropic/Bedrock.
func TestSanitizeDanglingToolCallsOrphanAssistantToolCallGetsPlaceholder(t *testing.T) {
	resetSanitizerStatsForTest()
	messages := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "lookup",
				Arguments: "{}",
			},
		}}),
		schema.UserMessage("continue"),
	}

	got := sanitizeDanglingToolCalls(messages)
	if len(got) != 4 {
		t.Fatalf("want user + assistant + placeholder + user (4), got %d", len(got))
	}
	if got[1].Role != schema.Assistant || len(got[1].ToolCalls) != 1 {
		t.Fatalf("assistant tool_use should be preserved, got %#v", got[1])
	}
	if got[2].Role != schema.Tool || got[2].ToolCallID != "call-1" {
		t.Fatalf("synthetic tool_result for call-1 missing, got %#v", got[2])
	}
	if got[3].Role != schema.User || got[3].Content != "continue" {
		t.Fatalf("trailing user message should pass through, got %#v", got[3])
	}
	if stats := GetSanitizerStats(); stats.Synthesized != 1 {
		t.Fatalf("expected synthesized=1, got %+v", stats)
	}
}

// TestSanitizeDanglingToolCallsTrailingBedrockID reproduces the observed
// production failure: a Bedrock-format tool_use ID (toolu_bdrk_...) is the
// last assistant message with no tool_result following. Sanitizer must
// repair the pairing so the downstream Bedrock relay does not 400 with
// "tool_use ids were found without tool_result blocks immediately after".
// Preferred recovery is synthesize-and-keep: assistant message stays
// intact and a placeholder tool_result is appended.
func TestSanitizeDanglingToolCallsTrailingBedrockID(t *testing.T) {
	resetSanitizerStatsForTest()
	messages := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("thinking", []schema.ToolCall{{
			ID:   "toolu_bdrk_01K1FBoWGTrTWzBf923Kz1Ua",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "lookup",
				Arguments: "{}",
			},
		}}),
	}

	got := sanitizeDanglingToolCalls(messages)
	if len(got) != 3 {
		t.Fatalf("want 3 messages (user + assistant tool_use + synthetic tool_result), got %d", len(got))
	}
	if len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ID != "toolu_bdrk_01K1FBoWGTrTWzBf923Kz1Ua" {
		t.Fatalf("assistant tool_use should be preserved verbatim, got %#v", got[1].ToolCalls)
	}
	if got[2].Role != schema.Tool || got[2].ToolCallID != "toolu_bdrk_01K1FBoWGTrTWzBf923Kz1Ua" {
		t.Fatalf("expected synthetic tool_result message, got role=%s id=%q", got[2].Role, got[2].ToolCallID)
	}
	if got[2].ToolName != "lookup" {
		t.Fatalf("placeholder should carry tool_name lookup, got %q", got[2].ToolName)
	}

	stats := GetSanitizerStats()
	if stats.Synthesized != 1 {
		t.Fatalf("expected synthesized=1, got %+v", stats)
	}
	if stats.Stripped != 0 {
		t.Fatalf("did not expect strip fallback, got %+v", stats)
	}
}

// TestSanitizeDanglingToolCallsPartialResultsSynthesize covers the case
// where one of multiple tool_calls already has a real result and only the
// missing ones need a placeholder.
func TestSanitizeDanglingToolCallsPartialResultsSynthesize(t *testing.T) {
	resetSanitizerStatsForTest()
	messages := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("dispatching", []schema.ToolCall{
			{
				ID:   "call-A",
				Type: "function",
				Function: schema.FunctionCall{Name: "lookup", Arguments: "{}"},
			},
			{
				ID:   "call-B",
				Type: "function",
				Function: schema.FunctionCall{Name: "fetch", Arguments: "{}"},
			},
		}),
		schema.ToolMessage("real result for A", "call-A", schema.WithToolName("lookup")),
		schema.UserMessage("continue"),
	}

	got := sanitizeDanglingToolCalls(messages)
	// Expected layout: user, assistant(tool_use A+B), tool(real A),
	// tool(synthetic B), user.
	if len(got) != 5 {
		t.Fatalf("want 5 sanitized messages, got %d", len(got))
	}
	if len(got[1].ToolCalls) != 2 {
		t.Fatalf("both tool_calls should be preserved, got %d", len(got[1].ToolCalls))
	}
	if got[2].Role != schema.Tool || got[2].ToolCallID != "call-A" || got[2].Content != "real result for A" {
		t.Fatalf("real tool result A should pass through, got %#v", got[2])
	}
	if got[3].Role != schema.Tool || got[3].ToolCallID != "call-B" {
		t.Fatalf("placeholder for call-B missing, got %#v", got[3])
	}
	if got[4].Role != schema.User || got[4].Content != "continue" {
		t.Fatalf("trailing user should remain, got %#v", got[4])
	}

	stats := GetSanitizerStats()
	if stats.Synthesized != 1 {
		t.Fatalf("expected synthesized=1 (only call-B), got %+v", stats)
	}
}

// TestSanitizerStatsCountsStripFallback ensures the legacy strip path also
// updates counters when synthesize isn't possible (missing IDs).
func TestSanitizerStatsCountsStripFallback(t *testing.T) {
	resetSanitizerStatsForTest()
	messages := []*schema.Message{
		schema.UserMessage("hello"),
		// An assistant tool_use with an empty ID forces the strip fallback.
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "",
			Type: "function",
			Function: schema.FunctionCall{Name: "lookup", Arguments: "{}"},
		}}),
		schema.UserMessage("continue"),
	}

	_ = sanitizeDanglingToolCalls(messages)
	stats := GetSanitizerStats()
	if stats.Stripped != 1 {
		t.Fatalf("expected stripped=1, got %+v", stats)
	}
	if stats.Synthesized != 0 {
		t.Fatalf("did not expect synthesize on missing-ID path, got %+v", stats)
	}
}

// Removed: TestSanitizeDanglingToolCallsConvertsIncompleteToolResults — the
// old strip-and-flatten path is superseded by synthesize-and-keep. See
// TestSanitizeDanglingToolCallsPartialResultsSynthesize for the equivalent
// scenario under the new behavior.
