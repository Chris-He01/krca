package hub

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type llmTraceContextKey struct{}

type llmTraceContext struct {
	SessionID string
	RunID     string
	UserID    string
}

// LLMRoundRecord is the complete request/response pair for one chat-model call.
type LLMRoundRecord struct {
	SessionID string             `json:"session_id,omitempty"`
	RunID     string             `json:"run_id"`
	UserID    string             `json:"user_id,omitempty"`
	AgentName string             `json:"agent_name"`
	Round     int64              `json:"round"`
	StartedAt time.Time          `json:"started_at"`
	Duration  time.Duration      `json:"duration_ns"`
	Streaming bool               `json:"streaming"`
	Request   []*schema.Message  `json:"request"`
	Tools     []*schema.ToolInfo `json:"tools,omitempty"`
	Response  *schema.Message    `json:"response,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// LLMRoundSink persists one completed LLM round.
type LLMRoundSink func(context.Context, LLMRoundRecord) error

type llmRoundRecorder struct {
	sink     LLMRoundSink
	counters sync.Map
}

func newLLMRoundRecorder(sink LLMRoundSink) *llmRoundRecorder {
	if sink == nil {
		return nil
	}
	return &llmRoundRecorder{sink: sink}
}

func (r *llmRoundRecorder) record(ctx context.Context, record LLMRoundRecord) {
	if r == nil || r.sink == nil {
		return
	}
	trace, _ := ctx.Value(llmTraceContextKey{}).(llmTraceContext)
	record.SessionID = trace.SessionID
	record.RunID = trace.RunID
	record.UserID = trace.UserID
	key := record.RunID + "\x00" + record.AgentName
	counter, _ := r.counters.LoadOrStore(key, &atomic.Int64{})
	record.Round = counter.(*atomic.Int64).Add(1)
	if err := r.sink(ctx, record); err != nil {
		log.Printf("[llm-trace] persist failed run_id=%s agent=%s round=%d: %v",
			record.RunID, record.AgentName, record.Round, err)
	}
}

func withLLMTraceContext(ctx context.Context, sessionID, runID, userID string) context.Context {
	return context.WithValue(ctx, llmTraceContextKey{}, llmTraceContext{
		SessionID: sessionID,
		RunID:     runID,
		UserID:    userID,
	})
}

type recordingChatModel struct {
	inner     model.ToolCallingChatModel
	agentName string
	recorder  *llmRoundRecorder
	tools     []*schema.ToolInfo
}

func withLLMRecording(inner model.ToolCallingChatModel, agentName string, recorder *llmRoundRecorder) model.ToolCallingChatModel {
	if inner == nil || recorder == nil {
		return inner
	}
	return &recordingChatModel{inner: inner, agentName: agentName, recorder: recorder}
}

func (m *recordingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	started := time.Now()
	input = sanitizeDanglingToolCalls(input)
	request := cloneMessages(input)
	response, err := m.inner.Generate(ctx, input, opts...)
	record := LLMRoundRecord{
		AgentName: m.agentName,
		StartedAt: started.UTC(),
		Duration:  time.Since(started),
		Request:   request,
		Tools:     cloneTools(m.tools),
		Response:  cloneMessage(response),
	}
	if err != nil {
		record.Error = err.Error()
	}
	m.recorder.record(ctx, record)
	return response, err
}

func (m *recordingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	started := time.Now()
	input = sanitizeDanglingToolCalls(input)
	request := cloneMessages(input)
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		m.recorder.record(ctx, LLMRoundRecord{
			AgentName: m.agentName,
			StartedAt: started.UTC(),
			Duration:  time.Since(started),
			Streaming: true,
			Request:   request,
			Tools:     cloneTools(m.tools),
			Error:     err.Error(),
		})
		return nil, err
	}

	copies := stream.Copy(2)
	go func() {
		response, streamErr := schema.ConcatMessageStream(copies[1])
		record := LLMRoundRecord{
			AgentName: m.agentName,
			StartedAt: started.UTC(),
			Duration:  time.Since(started),
			Streaming: true,
			Request:   request,
			Tools:     cloneTools(m.tools),
			Response:  cloneMessage(response),
		}
		if streamErr != nil {
			record.Error = streamErr.Error()
		}
		m.recorder.record(ctx, record)
	}()
	return copies[0], nil
}

func (m *recordingChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &recordingChatModel{
		inner:     inner,
		agentName: m.agentName,
		recorder:  m.recorder,
		tools:     cloneTools(tools),
	}, nil
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	if messages == nil {
		return nil
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return messages
	}
	var cloned []*schema.Message
	if err := json.Unmarshal(data, &cloned); err != nil {
		return messages
	}
	return cloned
}

func cloneMessage(message *schema.Message) *schema.Message {
	if message == nil {
		return nil
	}
	cloned := cloneMessages([]*schema.Message{message})
	if len(cloned) == 0 {
		return nil
	}
	return cloned[0]
}

func cloneTools(tools []*schema.ToolInfo) []*schema.ToolInfo {
	if tools == nil {
		return nil
	}
	data, err := json.Marshal(tools)
	if err != nil {
		return tools
	}
	var cloned []*schema.ToolInfo
	if err := json.Unmarshal(data, &cloned); err != nil {
		return tools
	}
	return cloned
}
