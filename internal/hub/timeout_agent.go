package hub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// stageBudgetAgent bounds one agent stage. Reaching the configured timeout or
// iteration limit is treated as a stage boundary rather than a fatal run error:
// the current findings are returned to the parent so it can continue.
type stageBudgetAgent struct {
	agent         adk.Agent
	timeout       time.Duration
	maxIterations int
	finalizer     adk.Agent
}

const (
	maxFinalizerEvidenceChars = 30000
	maxEvidenceItemChars      = 8000
)

func withAgentStageBudget(agent adk.Agent, timeout time.Duration, maxIterations int) adk.Agent {
	if agent == nil || (timeout <= 0 && maxIterations <= 0) {
		return agent
	}
	return &stageBudgetAgent{
		agent:         agent,
		timeout:       timeout,
		maxIterations: maxIterations,
	}
}

// withAgentFinalizer gives the root agent a separate best-effort summarization
// pass after its investigation budget is exhausted.
func withAgentFinalizer(agent adk.Agent, timeout time.Duration, maxIterations int, finalizer adk.Agent) adk.Agent {
	if agent == nil {
		return nil
	}
	return &stageBudgetAgent{
		agent:         agent,
		timeout:       timeout,
		maxIterations: maxIterations,
		finalizer:     finalizer,
	}
}

func (a *stageBudgetAgent) Name(ctx context.Context) string {
	return a.agent.Name(ctx)
}

func (a *stageBudgetAgent) Description(ctx context.Context) string {
	return a.agent.Description(ctx)
}

func (a *stageBudgetAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	return a.runWithBudget(ctx, func(runCtx context.Context) *adk.AsyncIterator[*adk.AgentEvent] {
		return a.agent.Run(runCtx, input, opts...)
	})
}

func (a *stageBudgetAgent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	resumable, ok := a.agent.(adk.ResumableAgent)
	if !ok {
		return errorIterator(fmt.Errorf("agent %q is not resumable", a.Name(ctx)))
	}
	return a.runWithBudget(ctx, func(runCtx context.Context) *adk.AsyncIterator[*adk.AgentEvent] {
		return resumable.Resume(runCtx, info, opts...)
	})
}

func (a *stageBudgetAgent) runWithBudget(
	ctx context.Context,
	start func(context.Context) *adk.AsyncIterator[*adk.AgentEvent],
) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	runCtx, cancel := context.WithCancel(ctx)
	if a.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, a.timeout)
	}
	source := start(runCtx)

	go func() {
		defer cancel()
		defer generator.Close()

		events := make(chan *adk.AgentEvent)
		var evidence []string
		evidenceChars := 0
		seenEvidence := make(map[string]struct{})
		go func() {
			defer close(events)
			for {
				event, ok := source.Next()
				if !ok {
					return
				}
				select {
				case events <- event:
				case <-runCtx.Done():
					return
				}
			}
		}()

		for {
			select {
			case event, ok := <-events:
				if !ok {
					if runCtx.Err() != nil {
						a.handleContextEnd(ctx, runCtx.Err(), evidence, generator)
					}
					return
				}
				if event != nil && isIterationLimit(event.Err) {
					cancel()
					a.handleStageLimit(ctx, stageLimitIterations, evidence, generator)
					return
				}
				if event != nil &&
					event.Err != nil &&
					ctx.Err() == nil &&
					runCtx.Err() == context.DeadlineExceeded {
					a.handleStageLimit(ctx, stageLimitTimeout, evidence, generator)
					return
				}
				collectStageEvidence(event, &evidence, &evidenceChars, seenEvidence)
				generator.Send(event)
			case <-runCtx.Done():
				a.handleContextEnd(ctx, runCtx.Err(), evidence, generator)
				return
			}
		}
	}()

	return iterator
}

type stageLimitKind int

const (
	stageLimitTimeout stageLimitKind = iota
	stageLimitIterations
)

func isIterationLimit(err error) bool {
	return err != nil && errors.Is(err, adk.ErrExceedMaxIterations)
}

func (a *stageBudgetAgent) handleContextEnd(
	parent context.Context,
	err error,
	evidence []string,
	generator *adk.AsyncGenerator[*adk.AgentEvent],
) {
	if parent.Err() != nil || err != context.DeadlineExceeded {
		generator.Send(&adk.AgentEvent{AgentName: a.Name(parent), Err: err})
		return
	}
	a.handleStageLimit(parent, stageLimitTimeout, evidence, generator)
}

func (a *stageBudgetAgent) handleStageLimit(
	ctx context.Context,
	kind stageLimitKind,
	evidence []string,
	generator *adk.AsyncGenerator[*adk.AgentEvent],
) {
	message := a.stageLimitMessage(ctx, kind)
	status := adk.EventFromMessage(schema.AssistantMessage(message, nil), nil, schema.Assistant, "")
	status.AgentName = a.Name(ctx)
	generator.Send(status)

	if a.finalizer == nil {
		return
	}

	evidenceText := strings.Join(evidence, "\n\n")
	prompt := fmt.Sprintf(
		"系统检测到顶层诊断%s。请停止继续采集新数据。优先根据下方 <collected_evidence> 中已保留的"+
			"子 Agent 结论进行总结；如需补充材料，可使用 list_dir(\".\") 查看当前用户工作区，并用相对路径读取文件。"+
			"基于当前已经获得的证据生成完整的阶段性诊断总结。必须明确区分已确认结论、推测和未完成项，"+
			"并给出下一步建议；不要因为文件为空或材料不完整而只返回错误。\n\n"+
			"触发信息：%s\n\n<collected_evidence>\n%s\n</collected_evidence>",
		a.limitDescription(kind),
		message,
		evidenceText,
	)
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: a.finalizer, EnableStreaming: false})
	iter := runner.Query(ctx, prompt)
	finalContentSeen := false
	for {
		event, ok := iter.Next()
		if !ok {
			if !finalContentSeen {
				generator.Send(a.fallbackSummaryEvent(ctx, message, evidence, nil))
			}
			return
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			generator.Send(a.fallbackSummaryEvent(ctx, message, evidence, event.Err))
			return
		}
		if msg, _, err := adk.GetMessage(event); err == nil &&
			msg != nil &&
			msg.Role == schema.Assistant &&
			strings.TrimSpace(msg.Content) != "" {
			finalContentSeen = true
		}
		generator.Send(event)
	}
}

func collectStageEvidence(
	event *adk.AgentEvent,
	evidence *[]string,
	evidenceChars *int,
	seen map[string]struct{},
) {
	if event == nil || *evidenceChars >= maxFinalizerEvidenceChars {
		return
	}
	msg, _, err := adk.GetMessage(event)
	if err != nil || msg == nil || msg.Role != schema.Assistant ||
		msg.ToolCallID != "" || len(msg.ToolCalls) > 0 {
		return
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" || IsStageLimitStatus(content) {
		return
	}
	key := event.AgentName + "\x00" + content
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	if len(content) > maxEvidenceItemChars {
		content = content[:maxEvidenceItemChars] + "\n...(truncated)"
	}
	item := fmt.Sprintf("[%s]\n%s", event.AgentName, content)
	remaining := maxFinalizerEvidenceChars - *evidenceChars
	if len(item) > remaining {
		item = item[:remaining]
	}
	*evidence = append(*evidence, item)
	*evidenceChars += len(item)
}

func (a *stageBudgetAgent) fallbackSummaryEvent(
	ctx context.Context,
	message string,
	evidence []string,
	finalizerErr error,
) *adk.AgentEvent {
	var fallback strings.Builder
	fallback.WriteString(message)
	if finalizerErr != nil {
		fallback.WriteString(fmt.Sprintf("\n\n收尾总结阶段执行失败：%v", finalizerErr))
	} else {
		fallback.WriteString("\n\n收尾总结未生成有效内容，以下保留超时前已经获得的结论：")
	}
	if len(evidence) > 0 {
		fallback.WriteString("\n\n")
		fallback.WriteString(strings.Join(evidence, "\n\n"))
	}
	event := adk.EventFromMessage(
		schema.AssistantMessage(fallback.String(), nil),
		nil,
		schema.Assistant,
		"",
	)
	event.AgentName = a.Name(ctx)
	return event
}

// IsStageLimitStatus reports whether content is the informational status event
// emitted when an agent stage reaches its configured budget.
func IsStageLimitStatus(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "当前阶段（") &&
		(strings.Contains(content, "已达到运行时长上限") ||
			strings.Contains(content, "已达到最大运行轮次"))
}

func (a *stageBudgetAgent) stageLimitMessage(ctx context.Context, kind stageLimitKind) string {
	name := a.Name(ctx)
	switch kind {
	case stageLimitIterations:
		if a.maxIterations > 0 {
			return fmt.Sprintf(
				"当前阶段（%s）已达到最大运行轮次 %d。系统将基于该阶段已有结果进行总结，并继续进入下一步。",
				name,
				a.maxIterations,
			)
		}
		return fmt.Sprintf(
			"当前阶段（%s）已达到最大运行轮次。系统将基于该阶段已有结果进行总结，并继续进入下一步。",
			name,
		)
	default:
		return fmt.Sprintf(
			"当前阶段（%s）已达到运行时长上限 %s。系统将基于该阶段已有结果进行总结，并继续进入下一步。",
			name,
			a.timeout,
		)
	}
}

func (a *stageBudgetAgent) limitDescription(kind stageLimitKind) string {
	if kind == stageLimitIterations {
		return "达到最大运行轮次"
	}
	return "达到运行时长上限"
}

func errorIterator(err error) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	generator.Send(&adk.AgentEvent{Err: err})
	generator.Close()
	return iterator
}
