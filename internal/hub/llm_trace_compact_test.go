package hub

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestCompactLLMRoundBoundsRepeatedContext(t *testing.T) {
	request := make([]*schema.Message, 12)
	for i := range request {
		request[i] = schema.UserMessage(strings.Repeat("上下文", 3000))
	}
	response := schema.AssistantMessage(strings.Repeat("结果", 5000), nil)
	record := LLMRoundRecord{
		RunID:     "run-compact",
		UserID:    "user-1",
		AgentName: "InsightSupervisor",
		Round:     9,
		StartedAt: time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC),
		Duration:  3 * time.Second,
		Request:   request,
		Tools: []*schema.ToolInfo{
			{Name: "append_journal", Desc: strings.Repeat("schema", 5000)},
		},
		Response: response,
	}

	got := CompactLLMRound(record)
	if got.RequestMessageCount != 12 {
		t.Fatalf("request count = %d, want 12", got.RequestMessageCount)
	}
	if len(got.RequestTail) != llmTraceRequestTailMessages {
		t.Fatalf("request tail = %d, want %d", len(got.RequestTail), llmTraceRequestTailMessages)
	}
	if !got.RequestTail[0].ContentTruncated || !got.Response.ContentTruncated {
		t.Fatal("expected long request and response content to be truncated")
	}
	if len(got.ToolNames) != 1 || got.ToolNames[0] != "append_journal" {
		t.Fatalf("unexpected tool names: %v", got.ToolNames)
	}
	if got.RequestBytes == 0 {
		t.Fatal("expected original request byte count")
	}
}
