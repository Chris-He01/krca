package hub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type autoApproveContextKey struct{}

// WithAutoApprove stores the per-run approval policy in context.
func WithAutoApprove(ctx context.Context, enabled bool) context.Context {
	v := enabled
	return context.WithValue(ctx, autoApproveContextKey{}, &v)
}

// AutoApproveFromContext returns the per-run approval policy if one was set.
func AutoApproveFromContext(ctx context.Context) (bool, bool) {
	value, ok := ctx.Value(autoApproveContextKey{}).(*bool)
	if !ok || value == nil {
		return false, false
	}
	return *value, true
}

type ApprovalTool struct {
	name string
	desc string
}

func NewApprovalTool() *ApprovalTool {
	return &ApprovalTool{
		name: "request_approval",
		desc: "Request human approval before continuing. Returns the approval input on resume.",
	}
}

func (t *ApprovalTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: t.desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"reason": {
				Type:     schema.String,
				Desc:     "Why approval is needed",
				Required: false,
			},
		}),
	}, nil
}

type approvalArgs struct {
	Reason string `json:"reason"`
}

func (t *ApprovalTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args approvalArgs
	_ = json.Unmarshal([]byte(argsJSON), &args)

	if autoApprove, ok := AutoApproveFromContext(ctx); ok && autoApprove {
		if args.Reason != "" {
			return fmt.Sprintf("approval auto-approved: %s", args.Reason), nil
		}
		return "approval auto-approved", nil
	}

	if wasInterrupted, _, _ := tool.GetInterruptState[any](ctx); !wasInterrupted {
		info := fmt.Sprintf("approval required: %s", args.Reason)
		return "", tool.Interrupt(ctx, info)
	}

	if isResumeFlow, hasResumeData, data := tool.GetResumeContext[string](ctx); isResumeFlow && hasResumeData {
		return fmt.Sprintf("approval received: %s", data), nil
	}

	return "approval received", nil
}

// ApprovalWrapper wraps any InvokableTool with an interrupt for human approval.
// On first call it interrupts with the tool name and arguments for review.
// On resume it executes the wrapped tool.
type ApprovalWrapper struct {
	inner tool.InvokableTool
}

func NewApprovalWrapper(inner tool.InvokableTool) *ApprovalWrapper {
	return &ApprovalWrapper{inner: inner}
}

func (w *ApprovalWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.inner.Info(ctx)
}

func (w *ApprovalWrapper) InvokableRun(ctx context.Context, argsJSON string, opts ...tool.Option) (string, error) {
	if autoApprove, ok := AutoApproveFromContext(ctx); ok && autoApprove {
		return w.inner.InvokableRun(ctx, argsJSON, opts...)
	}

	if wasInterrupted, _, _ := tool.GetInterruptState[any](ctx); !wasInterrupted {
		info, _ := w.inner.Info(ctx)
		name := "unknown"
		if info != nil {
			name = info.Name
		}
		return "", tool.Interrupt(ctx, fmt.Sprintf("tool [%s] requires approval.\nArguments: %s", name, argsJSON))
	}

	// Check if user rejected
	if isResumeFlow, hasData, data := tool.GetResumeContext[string](ctx); isResumeFlow && hasData {
		if data == "reject" || data == "denied" || data == "no" {
			return "execution rejected by user", nil
		}
	}

	return w.inner.InvokableRun(ctx, argsJSON, opts...)
}

// WrapToolsWithApproval wraps a slice of BaseTools with approval interrupts.
func WrapToolsWithApproval(tools []tool.BaseTool) []tool.BaseTool {
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		if invokable, ok := t.(tool.InvokableTool); ok {
			wrapped[i] = NewApprovalWrapper(invokable)
		} else {
			wrapped[i] = t
		}
	}
	return wrapped
}
