package hub

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
)

func TestDetectContextLengthErrorExtractsInputAndLimit(t *testing.T) {
	err := &openai.APIError{
		HTTPStatusCode: http.StatusBadRequest,
		Type:           "BadRequestError",
		Message:        "The input (36350 tokens) is longer than the model's context length (32768 tokens).",
	}

	got, ok := detectContextLengthError(err)
	if !ok {
		t.Fatal("expected context length error to be detected")
	}
	if got.InputTokens != 36350 || got.ContextLimit != 32768 {
		t.Fatalf("got input=%d limit=%d, want input=36350 limit=32768", got.InputTokens, got.ContextLimit)
	}
}

func TestDetectContextLengthErrorRejectsOrdinaryBadRequest(t *testing.T) {
	err := &openai.APIError{
		HTTPStatusCode: http.StatusBadRequest,
		Type:           "BadRequestError",
		Message:        "messages.1 has an invalid role",
	}
	if _, ok := detectContextLengthError(err); ok {
		t.Fatal("ordinary 400 must not be classified as context overflow")
	}
}

func TestCompactContextMessagesPreservesToolPairingAndLatestUser(t *testing.T) {
	largeResult := strings.Repeat("old diagnostic output ", 3000)
	messages := []*schema.Message{
		schema.SystemMessage("system instructions"),
		schema.UserMessage("old request"),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-old",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "inspect",
				Arguments: `{"target":"node-a"}`,
			},
		}}),
		schema.ToolMessage(largeResult, "call-old", schema.WithToolName("inspect")),
		schema.AssistantMessage("old conclusion", nil),
		schema.UserMessage("latest request must survive"),
	}

	got, stats := compactContextMessages(messages, &ContextLengthError{
		InputTokens:  50000,
		ContextLimit: 8000,
	}, 0, 0.85, 1)

	if stats.BytesAfter >= stats.BytesBefore {
		t.Fatalf("expected compaction to reduce bytes: before=%d after=%d", stats.BytesBefore, stats.BytesAfter)
	}
	if got[0].Role != schema.System || got[0].Content != "system instructions" {
		t.Fatalf("system message was not preserved: %#v", got[0])
	}
	if got[len(got)-1].Role != schema.User || got[len(got)-1].Content != "latest request must survive" {
		t.Fatalf("latest user message was not preserved: %#v", got[len(got)-1])
	}

	for i, msg := range got {
		if msg == nil || msg.Role != schema.Assistant || len(msg.ToolCalls) == 0 {
			continue
		}
		for _, call := range msg.ToolCalls {
			found := false
			for j := i + 1; j < len(got) && got[j] != nil && got[j].Role == schema.Tool; j++ {
				if got[j].ToolCallID == call.ID {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("tool call %q has no adjacent result after compaction", call.ID)
			}
		}
	}
}

func TestCompactContextMessagesShrinksOversizedLatestUserInput(t *testing.T) {
	latest := "request-start\n" + strings.Repeat("delegated diagnostic data ", 5000) + "\nrequest-end"
	messages := []*schema.Message{
		schema.SystemMessage("system instructions"),
		schema.UserMessage(latest),
	}

	got, stats := compactContextMessages(messages, &ContextLengthError{
		InputTokens:  50000,
		ContextLimit: 32768,
	}, 32768, 0.55, 1)

	if stats.BytesAfter >= stats.BytesBefore {
		t.Fatalf("expected latest input compaction to reduce bytes: before=%d after=%d", stats.BytesBefore, stats.BytesAfter)
	}
	if got[len(got)-1].Role != schema.User {
		t.Fatalf("latest message role = %s, want user", got[len(got)-1].Role)
	}
	content := got[len(got)-1].Content
	if !strings.Contains(content, "request-start") || !strings.Contains(content, "request-end") {
		t.Fatalf("compacted latest input lost its boundaries: %q", content)
	}
	if !strings.Contains(content, "[latest delegated input compacted]") {
		t.Fatalf("latest input was not marked compacted: %q", content)
	}
}

type contextRecoveryModel struct {
	calls    int
	requests [][]*schema.Message
}

func (m *contextRecoveryModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	m.requests = append(m.requests, cloneMessages(input))
	if m.calls == 1 {
		return nil, &openai.APIError{
			HTTPStatusCode: http.StatusBadRequest,
			Type:           "BadRequestError",
			Message:        "The input (36350 tokens) is longer than the model's context length (32768 tokens).",
		}
	}
	return schema.AssistantMessage("recovered", nil), nil
}

func (m *contextRecoveryModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *contextRecoveryModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func TestGenerateContextRecoveryCompactsRetriesAndEmitsProgress(t *testing.T) {
	fake := &contextRecoveryModel{}
	fm := &FailoverChatModel{
		contextCompactEnabled:    true,
		contextCompactTarget:     0.85,
		contextCompactMaxRetries: 2,
	}
	var statuses []string
	ctx := WithContextCompactionCallback(context.Background(), func(event ContextCompactionEvent) {
		statuses = append(statuses, event.Status)
	})
	messages := []*schema.Message{
		schema.SystemMessage("system"),
		schema.UserMessage("old"),
		schema.AssistantMessage(strings.Repeat("old details ", 5000), nil),
		schema.UserMessage("latest"),
	}

	got, err := fm.generateWithContextRecovery(ctx, fake, "qwen", 0, messages)
	if err != nil {
		t.Fatalf("generateWithContextRecovery returned error: %v", err)
	}
	if got.Content != "recovered" {
		t.Fatalf("response content = %q, want recovered", got.Content)
	}
	if fake.calls != 2 {
		t.Fatalf("model calls = %d, want 2", fake.calls)
	}
	if len(fake.requests) != 2 || messagesApproxBytes(fake.requests[1]) >= messagesApproxBytes(fake.requests[0]) {
		t.Fatal("second request was not compacted")
	}
	want := []string{"started", "retrying", "completed"}
	if strings.Join(statuses, ",") != strings.Join(want, ",") {
		t.Fatalf("progress statuses = %v, want %v", statuses, want)
	}
}
