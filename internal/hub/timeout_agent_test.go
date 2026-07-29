package hub

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type blockingAgent struct{}

func (blockingAgent) Name(context.Context) string        { return "InspectAgent" }
func (blockingAgent) Description(context.Context) string { return "test agent" }
func (blockingAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		<-ctx.Done()
	}()
	return iterator
}

type eventAgent struct{}

func (eventAgent) Name(context.Context) string        { return "EventAgent" }
func (eventAgent) Description(context.Context) string { return "test agent" }
func (eventAgent) Run(context.Context, *adk.AgentInput, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	generator.Send(&adk.AgentEvent{AgentName: "EventAgent"})
	generator.Close()
	return iterator
}

type iterationLimitAgent struct{}

func (iterationLimitAgent) Name(context.Context) string        { return "IterationAgent" }
func (iterationLimitAgent) Description(context.Context) string { return "test agent" }
func (iterationLimitAgent) Run(context.Context, *adk.AgentInput, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	return errorIterator(adk.ErrExceedMaxIterations)
}

type deadlineEventAgent struct{}

func (deadlineEventAgent) Name(context.Context) string        { return "DeadlineEventAgent" }
func (deadlineEventAgent) Description(context.Context) string { return "test agent" }
func (deadlineEventAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		<-ctx.Done()
		generator.Send(&adk.AgentEvent{Err: ctx.Err()})
	}()
	return iterator
}

type finalizerAgent struct{}

func (finalizerAgent) Name(context.Context) string        { return "SummaryAgent" }
func (finalizerAgent) Description(context.Context) string { return "test finalizer" }
func (finalizerAgent) Run(context.Context, *adk.AgentInput, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	event := adk.EventFromMessage(
		schema.AssistantMessage("基于当前证据生成的阶段性总结", nil),
		nil,
		schema.Assistant,
		"",
	)
	event.AgentName = "SummaryAgent"
	generator.Send(event)
	generator.Close()
	return iterator
}

type evidenceAgent struct{}

func (evidenceAgent) Name(context.Context) string        { return "InsightSupervisor" }
func (evidenceAgent) Description(context.Context) string { return "test evidence agent" }
func (evidenceAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		event := adk.EventFromMessage(
			schema.AssistantMessage("CloudStability 已确认主机运行健康，核心容器为 qwen36-27b。", nil),
			nil,
			schema.Assistant,
			"",
		)
		event.AgentName = "CloudStability"
		generator.Send(event)
		<-ctx.Done()
	}()
	return iterator
}

type capturingFinalizerAgent struct {
	prompt chan string
	empty  bool
}

func (a capturingFinalizerAgent) Name(context.Context) string        { return "SummaryAgent" }
func (a capturingFinalizerAgent) Description(context.Context) string { return "capturing finalizer" }
func (a capturingFinalizerAgent) Run(_ context.Context, input *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	if len(input.Messages) > 0 {
		a.prompt <- input.Messages[len(input.Messages)-1].Content
	}
	if !a.empty {
		event := adk.EventFromMessage(
			schema.AssistantMessage("最终阶段性总结", nil),
			nil,
			schema.Assistant,
			"",
		)
		event.AgentName = "SummaryAgent"
		generator.Send(event)
	}
	generator.Close()
	return iterator
}

func TestStageBudgetTurnsTimeoutIntoContinuationResult(t *testing.T) {
	agent := withAgentStageBudget(blockingAgent{}, 20*time.Millisecond, 10)
	start := time.Now()
	iterator := agent.Run(context.Background(), &adk.AgentInput{})

	event, ok := iterator.Next()
	if !ok || event == nil || event.Err != nil {
		t.Fatalf("expected timeout continuation event, got event=%v ok=%v", event, ok)
	}
	msg, _, err := adk.GetMessage(event)
	if err != nil || msg == nil || !strings.Contains(msg.Content, "已达到运行时长上限 20ms") {
		t.Fatalf("unexpected timeout message: msg=%v err=%v", msg, err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestStageBudgetForwardsEvents(t *testing.T) {
	agent := withAgentStageBudget(eventAgent{}, time.Second, 10)
	iterator := agent.Run(context.Background(), &adk.AgentInput{})

	event, ok := iterator.Next()
	if !ok || event == nil {
		t.Fatalf("expected forwarded event, got event=%v ok=%v", event, ok)
	}
	if event.AgentName != "EventAgent" || event.Err != nil {
		t.Fatalf("unexpected forwarded event: %+v", event)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("expected iterator to close")
	}
}

func TestStageBudgetTurnsIterationLimitIntoContinuationResult(t *testing.T) {
	agent := withAgentStageBudget(iterationLimitAgent{}, time.Second, 7)
	event, ok := agent.Run(context.Background(), &adk.AgentInput{}).Next()
	if !ok || event == nil || event.Err != nil {
		t.Fatalf("expected iteration continuation event, got event=%v ok=%v", event, ok)
	}
	msg, _, err := adk.GetMessage(event)
	if err != nil || msg == nil || !strings.Contains(msg.Content, "已达到最大运行轮次 7") {
		t.Fatalf("unexpected iteration message: msg=%v err=%v", msg, err)
	}
}

func TestStageBudgetConvertsDeadlineErrorEvent(t *testing.T) {
	agent := withAgentStageBudget(deadlineEventAgent{}, 20*time.Millisecond, 7)
	event, ok := agent.Run(context.Background(), &adk.AgentInput{}).Next()
	if !ok || event == nil || event.Err != nil {
		t.Fatalf("expected timeout continuation event, got event=%v ok=%v", event, ok)
	}
	msg, _, err := adk.GetMessage(event)
	if err != nil || msg == nil || !strings.Contains(msg.Content, "已达到运行时长上限") {
		t.Fatalf("unexpected timeout message: msg=%v err=%v", msg, err)
	}
}

func TestRootBudgetRunsFinalizerAfterTimeout(t *testing.T) {
	agent := withAgentFinalizer(blockingAgent{}, 20*time.Millisecond, 10, finalizerAgent{})
	iterator := agent.Run(context.Background(), &adk.AgentInput{})

	status, ok := iterator.Next()
	if !ok || status == nil || status.Err != nil {
		t.Fatalf("expected root limit status, got event=%v ok=%v", status, ok)
	}
	summary, ok := iterator.Next()
	if !ok || summary == nil || summary.Err != nil {
		t.Fatalf("expected finalizer result, got event=%v ok=%v", summary, ok)
	}
	msg, _, err := adk.GetMessage(summary)
	if err != nil || msg == nil || msg.Content != "基于当前证据生成的阶段性总结" {
		t.Fatalf("unexpected finalizer result: msg=%v err=%v", msg, err)
	}
}

func TestRootBudgetPassesCollectedEvidenceToFinalizer(t *testing.T) {
	prompts := make(chan string, 1)
	agent := withAgentFinalizer(
		evidenceAgent{},
		20*time.Millisecond,
		10,
		capturingFinalizerAgent{prompt: prompts},
	)
	iterator := agent.Run(context.Background(), &adk.AgentInput{})
	for {
		if _, ok := iterator.Next(); !ok {
			break
		}
	}
	prompt := <-prompts
	if !strings.Contains(prompt, "[CloudStability]") ||
		!strings.Contains(prompt, "核心容器为 qwen36-27b") {
		t.Fatalf("finalizer prompt missing collected evidence:\n%s", prompt)
	}
	if !strings.Contains(prompt, `list_dir(".")`) {
		t.Fatalf("finalizer prompt must use workspace-relative paths:\n%s", prompt)
	}
}

func TestRootBudgetFallsBackToCollectedEvidenceWhenFinalizerIsEmpty(t *testing.T) {
	prompts := make(chan string, 1)
	agent := withAgentFinalizer(
		evidenceAgent{},
		20*time.Millisecond,
		10,
		capturingFinalizerAgent{prompt: prompts, empty: true},
	)
	iterator := agent.Run(context.Background(), &adk.AgentInput{})
	var lastContent string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		msg, _, _ := adk.GetMessage(event)
		if msg != nil && strings.TrimSpace(msg.Content) != "" {
			lastContent = msg.Content
		}
	}
	if !strings.Contains(lastContent, "收尾总结未生成有效内容") ||
		!strings.Contains(lastContent, "核心容器为 qwen36-27b") {
		t.Fatalf("fallback did not preserve collected evidence:\n%s", lastContent)
	}
}

func TestIsStageLimitStatus(t *testing.T) {
	if !IsStageLimitStatus("当前阶段（InsightSupervisor）已达到运行时长上限 5m0s。系统将继续。") {
		t.Fatal("timeout status not recognized")
	}
	if IsStageLimitStatus("主机诊断总结：当前运行正常") {
		t.Fatal("normal conclusion recognized as status")
	}
}

func TestCollectResultDoesNotReplaceOutputWithStageLimitStatus(t *testing.T) {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	finding := adk.EventFromMessage(
		schema.AssistantMessage("主机诊断结论：节点健康，未发现异常。", nil),
		nil,
		schema.Assistant,
		"",
	)
	finding.AgentName = "CloudStability"
	status := adk.EventFromMessage(
		schema.AssistantMessage(
			"当前阶段（InspectAgent）已达到运行时长上限 20ms。系统将基于该阶段已有结果进行总结，并继续进入下一步。",
			nil,
		),
		nil,
		schema.Assistant,
		"",
	)
	status.AgentName = "InspectAgent"
	generator.Send(finding)
	generator.Send(status)
	generator.Close()

	result, err := collectResult("test-stage-limit", iterator)
	if err != nil {
		t.Fatalf("collectResult() error: %v", err)
	}
	if result.Output != "主机诊断结论：节点健康，未发现异常。" {
		t.Fatalf("output was replaced by stage status: %q", result.Output)
	}
	if len(result.Events) != 2 {
		t.Fatalf("events = %d, want both finding and status preserved", len(result.Events))
	}
}

func TestStageBudgetPreservesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	agent := withAgentStageBudget(blockingAgent{}, time.Second, 10)
	iterator := agent.Run(ctx, &adk.AgentInput{})
	cancel()

	event, ok := iterator.Next()
	if !ok || event == nil || !errors.Is(event.Err, context.Canceled) {
		t.Fatalf("expected parent cancellation error, got event=%v ok=%v", event, ok)
	}
}

func TestApplyScenePreservesExplicitAgentTimeout(t *testing.T) {
	cfg := Config{}
	scene := SceneConfig{
		Supervisor: AgentConfig{Name: "Supervisor", TimeoutSeconds: 300},
		SubAgents: []AgentConfig{
			{Name: "InspectAgent", TimeoutSeconds: 120},
			{Name: "AnalysisAgent"},
		},
	}
	scene.Runtime.TotalTimeoutSec = 300

	if err := cfg.ApplyScene(scene); err != nil {
		t.Fatalf("ApplyScene() error: %v", err)
	}
	if got := cfg.SubAgents[0].TimeoutSeconds; got != 120 {
		t.Fatalf("InspectAgent timeout = %d, want 120", got)
	}
	if got := cfg.SubAgents[1].TimeoutSeconds; got != 300 {
		t.Fatalf("AnalysisAgent timeout = %d, want scene default 300", got)
	}
}
