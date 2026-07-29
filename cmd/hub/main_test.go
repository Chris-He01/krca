package main

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"knsight-go/internal/hub"
)

func TestShouldUseAsRunOutput(t *testing.T) {
	if shouldUseAsRunOutput(schema.AssistantMessage("", nil)) {
		t.Fatal("empty assistant message must not replace output")
	}
	if shouldUseAsRunOutput(schema.AssistantMessage(
		"当前阶段（InsightSupervisor）已达到运行时长上限 5m0s。系统将基于已有结果总结。",
		nil,
	)) {
		t.Fatal("stage limit status must not replace output")
	}
	if !shouldUseAsRunOutput(schema.AssistantMessage("完整诊断结论", nil)) {
		t.Fatal("non-empty conclusion should replace output")
	}
	toolCallMessage := schema.AssistantMessage("正在调用工具", []schema.ToolCall{{
		Function: schema.FunctionCall{Name: "list_dir", Arguments: `{"path":"."}`},
	}})
	if shouldUseAsRunOutput(toolCallMessage) {
		t.Fatal("assistant tool call message must not replace output")
	}
	if shouldUseAsRunOutput(schema.UserMessage("用户消息")) {
		t.Fatal("non-assistant message must not replace output")
	}
}

func TestKnsightTodosJSONFromToolCallMessage(t *testing.T) {
	const want = `[{"id":1,"content":"采集数据","status":"in_progress"},{"id":2,"content":"总结","status":"pending"}]`
	msg := schema.AssistantMessage(
		"<!-- knsight-todos "+want+" -->\n正在委派 InspectAgent。",
		[]schema.ToolCall{{
			Function: schema.FunctionCall{
				Name:      "transfer_to_agent",
				Arguments: `{"agent_name":"InspectAgent"}`,
			},
		}},
	)

	if shouldUseAsRunOutput(msg) {
		t.Fatal("tool-call message must not replace final output")
	}
	if got := knsightTodosJSON(msg); got != want {
		t.Fatalf("todo metadata must still be extracted from tool-call message: got %q", got)
	}
}

func TestKnsightTodosJSONUsesLatestBlock(t *testing.T) {
	msg := schema.AssistantMessage(
		`<!-- knsight-todos [{"id":1,"content":"采集数据","status":"in_progress"}] -->
<!-- knsight-todos [{"id":1,"content":"采集数据","status":"completed"}] -->`,
		nil,
	)

	const want = `[{"id":1,"content":"采集数据","status":"completed"}]`
	if got := knsightTodosJSON(msg); got != want {
		t.Fatalf("expected latest todo block, got %q", got)
	}
}

func TestSanitizePublicEventContent(t *testing.T) {
	original := schema.AssistantMessage(
		`<!-- knsight-todos [{"id":1,"content":"采集数据","status":"in_progress"}] -->
正在委派 InspectAgent。`,
		nil,
	)
	event := hub.PublicEvent{Output: &hub.PublicOutput{Message: original}}

	sanitizePublicEventContent(&event)

	if got := event.Output.Message.Content; got != "正在委派 InspectAgent。" {
		t.Fatalf("unexpected public content: %q", got)
	}
	if original.Content == event.Output.Message.Content {
		t.Fatal("sanitization must use a copy instead of mutating the source message")
	}
}
