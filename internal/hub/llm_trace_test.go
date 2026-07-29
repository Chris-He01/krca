package hub

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type traceTestModel struct {
	response *schema.Message
	tools    []*schema.ToolInfo
}

func (m *traceTestModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return m.response, nil
}

func (m *traceTestModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{m.response}), nil
}

func (m *traceTestModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return &traceTestModel{response: m.response, tools: tools}, nil
}

func TestRecordingChatModelCapturesRound(t *testing.T) {
	var records []LLMRoundRecord
	recorder := newLLMRoundRecorder(func(_ context.Context, record LLMRoundRecord) error {
		records = append(records, record)
		return nil
	})
	base := &traceTestModel{response: schema.AssistantMessage("done", nil)}
	recording := withLLMRecording(base, "CloudStability", recorder)
	bound, err := recording.WithTools([]*schema.ToolInfo{{Name: "get_instance_metric_value", Desc: "query metrics"}})
	if err != nil {
		t.Fatalf("WithTools() error: %v", err)
	}

	ctx := withLLMTraceContext(context.Background(), "session-789", "run-123", "user-456")
	request := []*schema.Message{schema.UserMessage("query cpu")}
	response, err := bound.Generate(ctx, request)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if response.Content != "done" {
		t.Fatalf("response content = %q, want done", response.Content)
	}

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	got := records[0]
	if got.SessionID != "session-789" || got.RunID != "run-123" || got.UserID != "user-456" {
		t.Fatalf("trace context = %q/%q/%q", got.SessionID, got.RunID, got.UserID)
	}
	if got.AgentName != "CloudStability" || got.Round != 1 {
		t.Fatalf("agent round = %q/%d", got.AgentName, got.Round)
	}
	if len(got.Request) != 1 || got.Request[0].Content != "query cpu" {
		t.Fatalf("unexpected request: %+v", got.Request)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "get_instance_metric_value" {
		t.Fatalf("unexpected tools: %+v", got.Tools)
	}
	if got.Response == nil || got.Response.Content != "done" {
		t.Fatalf("unexpected response: %+v", got.Response)
	}
}
