package hub

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// SanitizingChatModel wraps a single ToolCallingChatModel and runs the same
// dangling-tool-call sanitizer that FailoverChatModel applies. Use this for
// non-failover model paths (external agents, dedicated openai endpoints)
// so they get the same protection against Bedrock/Claude 400s caused by an
// orphan tool_use block that has no matching tool_result.
type SanitizingChatModel struct {
	inner model.ToolCallingChatModel
}

// WrapSanitizing returns a SanitizingChatModel unless inner already provides
// the sanitization (e.g. *FailoverChatModel), in which case inner is returned
// unchanged so we don't double-process.
func WrapSanitizing(inner model.ToolCallingChatModel) model.ToolCallingChatModel {
	if inner == nil {
		return nil
	}
	if _, ok := inner.(*FailoverChatModel); ok {
		return inner
	}
	if _, ok := inner.(*SanitizingChatModel); ok {
		return inner
	}
	return &SanitizingChatModel{inner: inner}
}

func (s *SanitizingChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return s.inner.Generate(ctx, sanitizeDanglingToolCalls(messages), opts...)
}

func (s *SanitizingChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return s.inner.Stream(ctx, sanitizeDanglingToolCalls(messages), opts...)
}

func (s *SanitizingChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := s.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &SanitizingChatModel{inner: bound}, nil
}

// BindTools delegates to the inner model when it supports the legacy
// interface; otherwise it's a no-op.
func (s *SanitizingChatModel) BindTools(tools []*schema.ToolInfo) error {
	if cm, ok := s.inner.(interface {
		BindTools(tools []*schema.ToolInfo) error
	}); ok {
		return cm.BindTools(tools)
	}
	return nil
}
