package hub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	openai2 "github.com/meguminnnnnnnnn/go-openai"
)

// llmDebugBodyLimit caps how much of a non-2xx response body we tee into the
// log. Wanqing gateway errors fronting gemini come back as short JSON so 4 KiB
// is plenty; the cap is just a guard against accidentally logging a multi-MB
// HTML error page.
const llmDebugBodyLimit = 4096

// debugLoggingTransport wraps an http.RoundTripper and, on non-2xx responses
// (or transport errors), dumps the response status + first llmDebugBodyLimit
// bytes of the body to the logs. The body is then re-wrapped so the underlying
// openai client can still consume it normally. This exists because
// eino-ext/components/model/openai.convOrigAPIError strips the wrapping off
// upstream errors and only preserves APIError fields — when the gateway
// returns an error frame with empty fields (upstream provider does this),
// we end up with literally no diagnostic content. Hooking the transport gives
// us the raw bytes regardless of what the parser does with them.
type debugLoggingTransport struct {
	inner http.RoundTripper
	model string
}

func (d *debugLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := d.inner.RoundTrip(req)
	if err != nil {
		log.Printf("[llm-http] %s %s model=%s transport error after %s: %v",
			req.Method, req.URL.String(), d.model, time.Since(start), err)
		return resp, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, llmDebugBodyLimit+1))
	_ = resp.Body.Close()
	truncated := ""
	if len(body) > llmDebugBodyLimit {
		body = body[:llmDebugBodyLimit]
		truncated = " (truncated)"
	}
	log.Printf("[llm-http] %s %s model=%s status=%d ct=%q dur=%s body=%q%s",
		req.Method, req.URL.String(), d.model, resp.StatusCode,
		resp.Header.Get("Content-Type"), time.Since(start), string(body), truncated)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		log.Printf("[llm-http] %s %s model=%s body read error: %v", req.Method, req.URL.String(), d.model, readErr)
	}
	return resp, nil
}

// newDebugHTTPClient returns an http.Client that logs non-2xx response bodies
// for the given model. Disabled when LLM_HTTP_DEBUG=0 to allow opt-out.
func newDebugHTTPClient(modelID string, timeout time.Duration) *http.Client {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LLM_HTTP_DEBUG")), "0") {
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &debugLoggingTransport{inner: http.DefaultTransport, model: modelID},
	}
}

// describeErr renders an error for diagnostic logging. When err.Error() comes
// back blank — which happens with eino-ext's convOrigAPIError when the
// underlying APIError has both HTTPStatusCode=0 and Message="" (typical for
// gateway-relayed stream error frames that carry an empty message field) —
// we fall back to dumping the concrete type and struct fields so we can tell
// at all what the upstream actually returned.
func describeErr(err error) string {
	if err == nil {
		return "<nil>"
	}
	msg := err.Error()
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		// Always include the APIError fields explicitly; relying on
		// Error() loses Code/Type/Param when HTTPStatusCode==0.
		return fmt.Sprintf("%s (apiErr type=%T code=%v type_field=%q status=%d message=%q param=%v)",
			msg, apiErr, apiErr.Code, apiErr.Type, apiErr.HTTPStatusCode, apiErr.Message, apiErr.Param)
	}
	var rawAPIErr *openai2.APIError
	if errors.As(err, &rawAPIErr) && rawAPIErr != nil {
		return fmt.Sprintf("%s (apiErr type=%T code=%v type_field=%q status=%d message=%q param=%v)",
			msg, rawAPIErr, rawAPIErr.Code, rawAPIErr.Type, rawAPIErr.HTTPStatusCode, rawAPIErr.Message, rawAPIErr.Param)
	}
	var reqErr *openai2.RequestError
	if errors.As(err, &reqErr) && reqErr != nil {
		return fmt.Sprintf("%s (reqErr status=%d status_text=%q inner=%v body=%q)",
			msg, reqErr.HTTPStatusCode, reqErr.HTTPStatus, reqErr.Err, string(reqErr.Body))
	}
	if msg == "" {
		return fmt.Sprintf("(empty Error() — type=%T value=%+v)", err, err)
	}
	return msg
}

// FailoverChatModel wraps multiple OpenAI chat models with round-robin and failover.
// On each call it tries the current model; on error it rotates to the next.
// Fails only after trying all models.
type FailoverChatModel struct {
	models                   []model.ToolCallingChatModel
	names                    []string
	idx                      atomic.Int64
	rateLimitMaxRetries      int
	rateLimitWaitSeconds     int
	contextWindow            int
	contextCompactEnabled    bool
	contextCompactTarget     float64
	contextCompactMaxRetries int
}

func NewFailoverChatModel(ctx context.Context, cfg LLMConfig) (*FailoverChatModel, error) {
	modelIDs := cfg.Models()
	if len(modelIDs) == 0 {
		return nil, fmt.Errorf("no models configured")
	}

	baseURL := normalizeBaseURL(cfg.BaseURL)
	fm := &FailoverChatModel{
		models:                   make([]model.ToolCallingChatModel, 0, len(modelIDs)),
		names:                    modelIDs,
		rateLimitMaxRetries:      cfg.rateLimitMaxRetries(),
		rateLimitWaitSeconds:     cfg.rateLimitWaitSeconds(),
		contextWindow:            cfg.ContextWindow,
		contextCompactEnabled:    cfg.contextCompactEnabled(),
		contextCompactTarget:     cfg.contextCompactTarget(),
		contextCompactMaxRetries: cfg.contextCompactMaxRetries(),
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	for _, modelID := range modelIDs {
		chatCfg := &openai.ChatModelConfig{
			APIKey:     cfg.APIKey,
			Model:      modelID,
			BaseURL:    baseURL,
			HTTPClient: newDebugHTTPClient(modelID, timeout),
		}
		if cfg.MaxTokens > 0 {
			chatCfg.MaxTokens = &cfg.MaxTokens
		}
		m, err := openai.NewChatModel(ctx, chatCfg)
		if err != nil {
			return nil, fmt.Errorf("init model %s: %w", modelID, err)
		}
		fm.models = append(fm.models, m)
	}

	log.Printf("[llm] failover pool: %d models %v", len(fm.models), fm.names)
	return fm, nil
}

func (fm *FailoverChatModel) current() (int, model.ToolCallingChatModel) {
	n := int64(len(fm.models))
	raw := fm.idx.Load()
	i := int((raw%n + n) % n) // guard against negative modulo after int64 overflow
	return i, fm.models[i]
}

func (fm *FailoverChatModel) rotate() {
	fm.idx.Add(1)
}

// Per-endpoint inline retry budget for suspected-transient errors before we
// rotate to the next model. Two retries with exponential backoff (300ms,
// 600ms) is enough to ride out a flaky gateway / dropped SSE error frame
// without measurably hurting latency for permanent failures.
const (
	llmTransientMaxRetries  = 2
	llmTransientInitialWait = 300 * time.Millisecond
)

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr.HTTPStatusCode == http.StatusTooManyRequests
	}
	var rawAPIErr *openai2.APIError
	if errors.As(err, &rawAPIErr) && rawAPIErr != nil {
		return rawAPIErr.HTTPStatusCode == http.StatusTooManyRequests
	}
	var reqErr *openai2.RequestError
	if errors.As(err, &reqErr) && reqErr != nil {
		return reqErr.HTTPStatusCode == http.StatusTooManyRequests
	}
	return false
}

// isLikelyTransient reports whether err is plausibly a temporary upstream
// failure that's worth retrying on the same endpoint. The empty-fields
// APIError case (status=0 message="" code=nil) is the one we explicitly
// want to catch: it means we got an SSE error frame from the gateway with
// no diagnostic content, which is overwhelmingly a transient relay glitch
// rather than a permanent rejection.
func isLikelyTransient(err error) bool {
	if err == nil {
		return false
	}
	// Context errors are not retryable — caller has gone away.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Check RequestError first — it always carries the real HTTP status code
	// from the response. When the gateway returns a body whose error object is
	// missing the "message" field (e.g. {"error":{"type":"BadRequest"}}),
	// go-openai's UnmarshalJSON fails and handleErrorResp wraps the result in a
	// RequestError with the correct status. If we don't check this first, the
	// empty *APIError stored in RequestError.Err gets picked up by errors.As
	// below and its zero-value fields cause a false-positive transient match.
	var reqErr *openai2.RequestError
	if errors.As(err, &reqErr) && reqErr != nil {
		if reqErr.HTTPStatusCode >= 500 || reqErr.HTTPStatusCode == 429 || reqErr.HTTPStatusCode == 408 {
			return true
		}
		// HTTPStatusCode == 0 here means the transport never got a response —
		// dial error, RST, EOF mid-headers — all worth retrying.
		if reqErr.HTTPStatusCode == 0 {
			return true
		}
		return false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		// Stream error frame with empty fields — almost certainly a relay
		// glitch; the gateway gave us nothing to act on, so a retry is the
		// only reasonable next step.
		if apiErr.HTTPStatusCode == 0 && apiErr.Message == "" && apiErr.Code == nil && apiErr.Type == "" {
			return true
		}
		// Standard transient HTTP semantics.
		if apiErr.HTTPStatusCode >= 500 || apiErr.HTTPStatusCode == 429 || apiErr.HTTPStatusCode == 408 {
			return true
		}
		return false
	}
	var rawAPIErr *openai2.APIError
	if errors.As(err, &rawAPIErr) && rawAPIErr != nil {
		if rawAPIErr.HTTPStatusCode == 0 && rawAPIErr.Message == "" && rawAPIErr.Code == nil && rawAPIErr.Type == "" {
			return true
		}
		if rawAPIErr.HTTPStatusCode >= 500 || rawAPIErr.HTTPStatusCode == 429 || rawAPIErr.HTTPStatusCode == 408 {
			return true
		}
		return false
	}
	// Bare network/transport errors (no APIError / RequestError in the
	// chain) are conservatively treated as transient.
	return true
}

func (fm *FailoverChatModel) rateLimitWait() time.Duration {
	if fm.rateLimitWaitSeconds <= 0 {
		return time.Duration(defaultRateLimitWaitSeconds) * time.Second
	}
	return time.Duration(fm.rateLimitWaitSeconds) * time.Second
}

func (fm *FailoverChatModel) shouldRetryRateLimit(err error, attempts int) bool {
	return isRateLimitError(err) && attempts < fm.rateLimitMaxRetries
}

// sleepWithCtx waits for d but bails out early if ctx is cancelled.
func sleepWithCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Generate with failover: try current model, rotate on error, fail after all tried.
// If context is already cancelled, return immediately without trying all models.
func (fm *FailoverChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	messages = sanitizeDanglingToolCalls(messages)
	var lastErr error
	for tried := 0; tried < len(fm.models); tried++ {
		// Stop immediately if context is done (client disconnected / timeout)
		if ctx.Err() != nil {
			log.Printf("[llm] context cancelled, stopping failover after %d attempts: %v", tried, ctx.Err())
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			break
		}
		i, m := fm.current()
		result, err := fm.generateWithContextRecovery(ctx, m, fm.names[i], i, messages, opts...)
		if err == nil {
			return result, nil
		}
		lastErr = err
		log.Printf("[llm] model %s (#%d) Generate error: %s, rotating...", fm.names[i], i, describeErr(err))
		fm.rotate()
	}
	NotifyError("llm:failover", ErrorParams{
		Component: "LLM",
		Error:     fmt.Sprintf("all models failed: %s", describeErr(lastErr)),
	})
	return nil, fmt.Errorf("all %d models failed, last error: %s: %w", len(fm.models), describeErr(lastErr), lastErr)
}

func (fm *FailoverChatModel) generateWithContextRecovery(
	ctx context.Context, m model.ToolCallingChatModel, name string, idx int,
	messages []*schema.Message, opts ...model.Option,
) (*schema.Message, error) {
	current := messages
	result, err := fm.generateWithTransientRetry(ctx, m, name, idx, current, opts...)
	if err == nil || !fm.contextCompactEnabled || fm.contextCompactMaxRetries == 0 {
		return result, err
	}
	overflow, ok := detectContextLengthError(err)
	if !ok {
		return nil, err
	}

	emitContextCompaction(ctx, ContextCompactionEvent{
		Status:       "started",
		Model:        name,
		InputTokens:  overflow.InputTokens,
		ContextLimit: overflow.ContextLimit,
		Message:      contextCompactionStartedMessage(overflow),
	})

	for attempt := 1; attempt <= fm.contextCompactMaxRetries; attempt++ {
		compacted, stats := compactContextMessages(
			current, overflow, fm.contextWindow, fm.contextCompactTarget, attempt,
		)
		log.Printf("[llm] context overflow model=%s (#%d) input=%d limit=%d compact_attempt=%d messages=%d->%d bytes=%d->%d; retrying",
			name, idx, overflow.InputTokens, overflow.ContextLimit, attempt,
			stats.MessagesBefore, stats.MessagesAfter, stats.BytesBefore, stats.BytesAfter)
		emitContextCompaction(ctx, ContextCompactionEvent{
			Status:         "retrying",
			Model:          name,
			Attempt:        attempt,
			InputTokens:    overflow.InputTokens,
			ContextLimit:   overflow.ContextLimit,
			MessagesBefore: stats.MessagesBefore,
			MessagesAfter:  stats.MessagesAfter,
			BytesBefore:    stats.BytesBefore,
			BytesAfter:     stats.BytesAfter,
			Message:        fmt.Sprintf("已压缩较早的工具调用和对话记录，正在自动重试模型（第 %d 次）。", attempt),
		})

		result, err = fm.generateWithTransientRetry(ctx, m, name, idx, compacted, opts...)
		if err == nil {
			emitContextCompaction(ctx, ContextCompactionEvent{
				Status:         "completed",
				Model:          name,
				Attempt:        attempt,
				MessagesBefore: stats.MessagesBefore,
				MessagesAfter:  stats.MessagesAfter,
				BytesBefore:    stats.BytesBefore,
				BytesAfter:     stats.BytesAfter,
				Message:        "上下文压缩完成，模型已恢复处理。",
			})
			return result, nil
		}
		nextOverflow, stillOverflow := detectContextLengthError(err)
		if !stillOverflow {
			return nil, err
		}
		overflow = nextOverflow
		current = compacted
	}
	emitContextCompaction(ctx, ContextCompactionEvent{
		Status:       "failed",
		Model:        name,
		Attempt:      fm.contextCompactMaxRetries,
		InputTokens:  overflow.InputTokens,
		ContextLimit: overflow.ContextLimit,
		Message:      "自动压缩历史记录后请求仍超过模型上下文限制，将尝试其他模型。",
	})
	return nil, fmt.Errorf("context length exceeded after %d automatic compaction retries: %w",
		fm.contextCompactMaxRetries, err)
}

func contextCompactionStartedMessage(overflow *ContextLengthError) string {
	if overflow != nil && overflow.InputTokens > 0 && overflow.ContextLimit > 0 {
		return fmt.Sprintf("当前请求超过模型上下文限制（%d / %d tokens），接下来将压缩较早的工具调用和对话记录并自动重试。",
			overflow.InputTokens, overflow.ContextLimit)
	}
	return "当前请求超过模型上下文限制，接下来将压缩较早的工具调用和对话记录并自动重试。"
}

// generateWithTransientRetry calls m.Generate once and, if the error looks
// like a transient gateway glitch (empty-fields APIError / 5xx / 429
// / transport error), 429 error retries with a fixed interval (fm.rateLimitWait),
// capped at fm.rateLimitMaxRetries, others retries the same endpoint
// up to llmTransientMaxRetries times with exponential backoff.
// Permanent errors (4xx other than 408/429, ontext cancellation)
// propagate immediately so we move on to the next model without burning retries.
func (fm *FailoverChatModel) generateWithTransientRetry(
	ctx context.Context, m model.ToolCallingChatModel, name string, idx int,
	messages []*schema.Message, opts ...model.Option,
) (*schema.Message, error) {
	transientWait := llmTransientInitialWait
	rateLimitWait := fm.rateLimitWait()
	var lastErr error
	var transientAttempts, rateLimitAttempts int
	for {
		result, err := m.Generate(ctx, messages, opts...)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if isRateLimitError(err) {
			if !fm.shouldRetryRateLimit(err, rateLimitAttempts) {
				return nil, err
			}
			rateLimitAttempts++
			log.Printf("[llm] model %s (#%d) Generate rate-limited (attempt %d/%d, sleep=%s): %s",
				name, idx, rateLimitAttempts, fm.rateLimitMaxRetries, rateLimitWait, describeErr(err))
			if sleepErr := sleepWithCtx(ctx, rateLimitWait); sleepErr != nil {
				return nil, lastErr
			}
			continue
		}

		if transientAttempts == llmTransientMaxRetries || !isLikelyTransient(err) {
			return nil, err
		}
		transientAttempts++
		log.Printf("[llm] model %s (#%d) Generate transient error (attempt %d/%d, sleep=%s): %s",
			name, idx, transientAttempts, llmTransientMaxRetries, transientWait, describeErr(err))
		if sleepErr := sleepWithCtx(ctx, transientWait); sleepErr != nil {
			return nil, lastErr
		}
		transientWait *= 2
	}
}

// streamWithTransientRetry mirrors generateWithTransientRetry for the Stream
// path. Note: a successfully-returned StreamReader can still surface errors
// later during Recv; those happen inside the goroutine started by the adk
// runtime and are out of scope here — we can only retry the initial
// connection / handshake error.
func (fm *FailoverChatModel) streamWithTransientRetry(
	ctx context.Context, m model.ToolCallingChatModel, name string, idx int,
	messages []*schema.Message, opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	transientWait := llmTransientInitialWait
	rateLimitWait := fm.rateLimitWait()
	var lastErr error
	var transientAttempts, rateLimitAttempts int
	for {
		reader, err := m.Stream(ctx, messages, opts...)
		if err == nil {
			return reader, nil
		}
		lastErr = err

		if isRateLimitError(err) {
			if !fm.shouldRetryRateLimit(err, rateLimitAttempts) {
				return nil, err
			}
			rateLimitAttempts++
			log.Printf("[llm] model %s (#%d) Stream rate-limited (attempt %d/%d, sleep=%s): %s",
				name, idx, rateLimitAttempts, fm.rateLimitMaxRetries, rateLimitWait, describeErr(err))
			if sleepErr := sleepWithCtx(ctx, rateLimitWait); sleepErr != nil {
				return nil, lastErr
			}
			continue
		}

		if transientAttempts == llmTransientMaxRetries || !isLikelyTransient(err) {
			return nil, err
		}
		transientAttempts++
		log.Printf("[llm] model %s (#%d) Stream transient error (attempt %d/%d, sleep=%s): %s",
			name, idx, transientAttempts, llmTransientMaxRetries, transientWait, describeErr(err))
		if sleepErr := sleepWithCtx(ctx, transientWait); sleepErr != nil {
			return nil, lastErr
		}
		transientWait *= 2
	}
}

// Stream with failover: try current model, rotate on error, fail after all tried.
func (fm *FailoverChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	messages = sanitizeDanglingToolCalls(messages)
	var lastErr error
	for tried := 0; tried < len(fm.models); tried++ {
		if ctx.Err() != nil {
			log.Printf("[llm] context cancelled, stopping stream failover after %d attempts: %v", tried, ctx.Err())
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			break
		}
		i, m := fm.current()
		reader, err := fm.streamWithContextRecovery(ctx, m, fm.names[i], i, messages, opts...)
		if err == nil {
			return reader, nil
		}
		lastErr = err
		log.Printf("[llm] model %s (#%d) Stream error: %s, rotating...", fm.names[i], i, describeErr(err))
		fm.rotate()
	}
	NotifyError("llm:failover", ErrorParams{
		Component: "LLM",
		Error:     fmt.Sprintf("all models failed: %s", describeErr(lastErr)),
	})
	return nil, fmt.Errorf("all %d models failed, last error: %s: %w", len(fm.models), describeErr(lastErr), lastErr)
}

func (fm *FailoverChatModel) streamWithContextRecovery(
	ctx context.Context, m model.ToolCallingChatModel, name string, idx int,
	messages []*schema.Message, opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	current := messages
	reader, err := fm.streamWithTransientRetry(ctx, m, name, idx, current, opts...)
	if err == nil || !fm.contextCompactEnabled || fm.contextCompactMaxRetries == 0 {
		return reader, err
	}
	overflow, ok := detectContextLengthError(err)
	if !ok {
		return nil, err
	}
	emitContextCompaction(ctx, ContextCompactionEvent{
		Status:       "started",
		Model:        name,
		InputTokens:  overflow.InputTokens,
		ContextLimit: overflow.ContextLimit,
		Message:      contextCompactionStartedMessage(overflow),
	})
	for attempt := 1; attempt <= fm.contextCompactMaxRetries; attempt++ {
		compacted, stats := compactContextMessages(
			current, overflow, fm.contextWindow, fm.contextCompactTarget, attempt,
		)
		log.Printf("[llm] stream context overflow model=%s (#%d) input=%d limit=%d compact_attempt=%d messages=%d->%d bytes=%d->%d; retrying",
			name, idx, overflow.InputTokens, overflow.ContextLimit, attempt,
			stats.MessagesBefore, stats.MessagesAfter, stats.BytesBefore, stats.BytesAfter)
		emitContextCompaction(ctx, ContextCompactionEvent{
			Status:         "retrying",
			Model:          name,
			Attempt:        attempt,
			InputTokens:    overflow.InputTokens,
			ContextLimit:   overflow.ContextLimit,
			MessagesBefore: stats.MessagesBefore,
			MessagesAfter:  stats.MessagesAfter,
			BytesBefore:    stats.BytesBefore,
			BytesAfter:     stats.BytesAfter,
			Message:        fmt.Sprintf("已压缩较早的工具调用和对话记录，正在自动重试模型（第 %d 次）。", attempt),
		})
		reader, err = fm.streamWithTransientRetry(ctx, m, name, idx, compacted, opts...)
		if err == nil {
			emitContextCompaction(ctx, ContextCompactionEvent{
				Status:         "completed",
				Model:          name,
				Attempt:        attempt,
				MessagesBefore: stats.MessagesBefore,
				MessagesAfter:  stats.MessagesAfter,
				BytesBefore:    stats.BytesBefore,
				BytesAfter:     stats.BytesAfter,
				Message:        "上下文压缩完成，模型已恢复处理。",
			})
			return reader, nil
		}
		nextOverflow, stillOverflow := detectContextLengthError(err)
		if !stillOverflow {
			return nil, err
		}
		overflow = nextOverflow
		current = compacted
	}
	emitContextCompaction(ctx, ContextCompactionEvent{
		Status:       "failed",
		Model:        name,
		Attempt:      fm.contextCompactMaxRetries,
		InputTokens:  overflow.InputTokens,
		ContextLimit: overflow.ContextLimit,
		Message:      "自动压缩历史记录后请求仍超过模型上下文限制，将尝试其他模型。",
	})
	return nil, fmt.Errorf("context length exceeded after %d automatic compaction retries: %w",
		fm.contextCompactMaxRetries, err)
}

// sanitizeDanglingToolCalls keeps tool_use blocks paired with a tool_result.
// Anthropic/Bedrock 400s any assistant message whose tool_use isn't followed
// by a matching tool_result. When that pairing is broken we have two repair
// strategies and pick whichever keeps the conversation closer to the model's
// original intent:
//
//  1. **Synthesize-and-keep** (preferred): if every emitted tool_call has a
//     non-empty ID, append a placeholder schema.ToolMessage for each missing
//     result. The model then sees a clear "[tool result missing — call
//     interrupted]" and can recover gracefully without thinking the tool call
//     was wiped from history.
//  2. **Strip-and-flatten** (fallback): when an ID is missing entirely (the
//     stream cut off mid-emit so we can't bind a placeholder) we drop the
//     dangling ToolCalls and convert any orphan tool messages into annotated
//     user context.
//
// Standalone tool messages with no parent assistant tool_use are always
// converted to user context — there is no valid tool_use to bind them to.
//
// All paths increment package-level counters exposed via SanitizerStats so
// we can observe how often this fires in prod.
func sanitizeDanglingToolCalls(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}

	out := make([]*schema.Message, 0, len(messages))
	resolvedToolCallIDs := make(map[string]bool)
	var stripped, synthesized, orphanConverted, dedupDropped int

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg == nil {
			out = append(out, msg)
			continue
		}

		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			// j marks the end of the contiguous tool-result block that follows
			// this assistant message. All messages in [i+1, j) have role=Tool.
			j := i + 1
			for j < len(messages) && messages[j] != nil && messages[j].Role == schema.Tool {
				j++
			}

			seenInAssistant := make(map[string]bool)
			droppedReplayedIDs := make(map[string]bool) // only for cross-turn replays
			filteredToolCalls := make([]schema.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				if tc.ID == "" {
					// ID-less calls can't be deduplicated; pass through to the
					// strip-and-flatten logic below.
					filteredToolCalls = append(filteredToolCalls, tc)
					continue
				}
				if resolvedToolCallIDs[tc.ID] {
					// This ID was already resolved in an earlier assistant turn —
					// it is a cross-turn replay. Record it so we can skip the
					// matching result messages further below.
					droppedReplayedIDs[tc.ID] = true
					dedupDropped++
					continue
				}
				if seenInAssistant[tc.ID] {
					// Same ID appears more than once within this assistant turn.
					// Drop the duplicate; do NOT add to droppedReplayedIDs because
					// the result message (if any) may correspond to the kept copy.
					dedupDropped++
					continue
				}
				seenInAssistant[tc.ID] = true
				filteredToolCalls = append(filteredToolCalls, tc)
			}

			// If dedup removed any calls, produce a shallow clone of the assistant
			// message with the filtered ToolCalls slice so we don't mutate the
			// original input.
			workingMsg := msg
			if len(filteredToolCalls) != len(msg.ToolCalls) {
				cloned := *msg
				cloned.ToolCalls = filteredToolCalls
				workingMsg = &cloned
			}

			// --- Build the set of tool_result IDs present in [i+1, j) ---
			// Skip result messages that belong to cross-turn replayed IDs; those
			// results were already accounted for when the ID was first resolved.
			toolResults := make(map[string]bool, len(filteredToolCalls))
			for k := i + 1; k < j; k++ {
				tm := messages[k]
				if tm == nil {
					continue
				}
				// Skip results that pair with a cross-turn replayed (dropped) call.
				// Note: we only skip here for droppedReplayedIDs, not for
				// seenInAssistant duplicates — see the comment above.
				if tm.ToolCallID != "" && droppedReplayedIDs[tm.ToolCallID] {
					continue
				}
				if tm.ToolCallID != "" {
					toolResults[tm.ToolCallID] = true
				}
			}

			// Check whether every retained tool_call has a result.
			complete := true
			anyMissingID := false
			for _, tc := range workingMsg.ToolCalls {
				if tc.ID == "" {
					anyMissingID = true
					complete = false
					break
				}
				if !toolResults[tc.ID] {
					complete = false
				}
			}

			// --- Edge case: all tool_calls were deduped away ---
			// The assistant message has no remaining tool_calls. If it still has
			// text content, emit it as a plain assistant message. Any trailing
			// tool result messages are orphans; convert them to user context.
			if len(workingMsg.ToolCalls) == 0 {
				if workingMsg.Content != "" || workingMsg.ReasoningContent != "" || len(workingMsg.AssistantGenMultiContent) > 0 {
					cloned := *workingMsg
					cloned.ToolCalls = nil
					out = append(out, &cloned)
				}
				for k := i + 1; k < j; k++ {
					tm := messages[k]
					if tm == nil {
						continue
					}
					if tm.ToolCallID != "" && droppedReplayedIDs[tm.ToolCallID] {
						continue
					}
					if tm.ToolCallID != "" {
						resolvedToolCallIDs[tm.ToolCallID] = true
					}
					if converted := toolResultAsUserMessage(tm); converted != nil {
						out = append(out, converted)
						orphanConverted++
					}
				}
				i = j - 1
				continue
			}

			if complete {
				out = append(out, workingMsg)
				for k := i + 1; k < j; k++ {
					tm := messages[k]
					if tm == nil {
						continue
					}
					if tm.ToolCallID != "" && droppedReplayedIDs[tm.ToolCallID] {
						continue
					}
					// Ensure content is non-empty before forwarding (Gemini 400 guard).
					normalized := ensureNonEmptyToolContent(tm)
					out = append(out, normalized)
					if normalized.ToolCallID != "" {
						resolvedToolCallIDs[normalized.ToolCallID] = true
					}
				}
				i = j - 1
				continue
			}

			// Synthesize-and-keep path: every tool_call has an ID, so we can
			// preserve the assistant message verbatim and append a placeholder
			// result for whichever IDs are missing.
			if !anyMissingID {
				out = append(out, workingMsg)
				for k := i + 1; k < j; k++ {
					tm := messages[k]
					if tm == nil {
						continue
					}
					if tm.ToolCallID != "" && droppedReplayedIDs[tm.ToolCallID] {
						continue
					}
					normalized := ensureNonEmptyToolContent(tm)
					out = append(out, normalized)
					if normalized.ToolCallID != "" {
						resolvedToolCallIDs[normalized.ToolCallID] = true
					}
				}
				for _, tc := range workingMsg.ToolCalls {
					if toolResults[tc.ID] {
						continue // already emitted above
					}
					// Synthesize a "[tool result missing]" placeholder so the model
					// sees a well-formed turn and can decide to retry or move on.
					synthetic := syntheticToolResult(tc)
					out = append(out, synthetic)
					synthesized++
					if tc.ID != "" {
						resolvedToolCallIDs[tc.ID] = true
					}
				}
				i = j - 1
				continue
			}

			// Strip-and-flatten fallback: at least one tool_call has no ID, so
			// we cannot synthesize a binding placeholder. Drop the dangling
			// ToolCalls and convert any partial tool messages to user context.
			stripped++
			if workingMsg.Content != "" || workingMsg.ReasoningContent != "" || len(workingMsg.AssistantGenMultiContent) > 0 {
				cloned := *workingMsg
				cloned.ToolCalls = nil
				out = append(out, &cloned)
			}
			for k := i + 1; k < j; k++ {
				tm := messages[k]
				if tm == nil {
					continue
				}
				if tm.ToolCallID != "" && droppedReplayedIDs[tm.ToolCallID] {
					continue
				}
				if tm.ToolCallID != "" {
					resolvedToolCallIDs[tm.ToolCallID] = true
				}
				if converted := toolResultAsUserMessage(tm); converted != nil {
					out = append(out, converted)
				}
			}
			i = j - 1
			continue
		}

		// --- Standalone tool message (no preceding assistant tool_call in output) ---
		// This happens when the assistant message was entirely stripped or when the
		// ADK runtime inserts a bare tool result. Convert to user context so the
		// history remains valid.
		if msg.Role == schema.Tool {
			if msg.ToolCallID != "" {
				if resolvedToolCallIDs[msg.ToolCallID] {
					// Already resolved in an earlier turn — skip to prevent duplicate
					// tool_result entries for the same ID.
					dedupDropped++
					continue
				}
				resolvedToolCallIDs[msg.ToolCallID] = true
			}
			orphanConverted++
			if converted := toolResultAsUserMessage(msg); converted != nil {
				out = append(out, converted)
			}
			continue
		}

		out = append(out, msg)
	}

	if stripped+synthesized+orphanConverted+dedupDropped > 0 {
		recordSanitizer(stripped, synthesized, orphanConverted)
		log.Printf("[llm] sanitizer ran: before=%d after=%d stripped=%d synthesized=%d orphan_converted=%d dedup_dropped=%d",
			len(messages), len(out), stripped, synthesized, orphanConverted, dedupDropped)
	}
	return out
}

// syntheticToolResult builds a placeholder schema.ToolMessage for a tool_call
// whose real result is missing. The message body is short, unambiguous, and
// tells the model the call was interrupted so it can retry or move on.
func syntheticToolResult(tc schema.ToolCall) *schema.Message {
	name := tc.Function.Name
	body := "[tool result missing — the previous tool call was interrupted or " +
		"never returned. Treat this call as failed; decide whether to retry " +
		"with the same arguments or to proceed without it.]"
	if name != "" {
		return schema.ToolMessage(body, tc.ID, schema.WithToolName(name))
	}
	return schema.ToolMessage(body, tc.ID)
}

// emptyToolResultPlaceholder is the content substituted when a tool message
// has an empty Content field. Gemini rejects tool_result parts with empty
// content (400 INVALID_ARGUMENT) when the paired tool_call carries a
// thought_signature, so we must always supply a non-empty body.
const emptyToolResultPlaceholder = "[empty tool result]"

// ensureNonEmptyToolContent returns the message unchanged if its Content is
// non-empty. When Content is empty (tool returned nothing / serialized as "")
// it returns a shallow clone with Content replaced by emptyToolResultPlaceholder
// so that Gemini never sees an empty tool_result body.
func ensureNonEmptyToolContent(msg *schema.Message) *schema.Message {
	if msg == nil || msg.Content != "" {
		return msg
	}
	cloned := *msg
	cloned.Content = emptyToolResultPlaceholder
	return &cloned
}

func toolResultAsUserMessage(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	name := msg.ToolName
	if name == "" {
		name = msg.ToolCallID
	}
	content := msg.Content
	if content == "" {
		content = emptyToolResultPlaceholder
	}
	if name != "" {
		content = fmt.Sprintf("[tool result: %s]\n%s", name, content)
	}
	return schema.UserMessage(content)
}

// WithTools returns a new FailoverChatModel with tools bound to all models.
func (fm *FailoverChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	newFm := &FailoverChatModel{
		models:                   make([]model.ToolCallingChatModel, len(fm.models)),
		names:                    fm.names,
		rateLimitMaxRetries:      fm.rateLimitMaxRetries,
		rateLimitWaitSeconds:     fm.rateLimitWaitSeconds,
		contextWindow:            fm.contextWindow,
		contextCompactEnabled:    fm.contextCompactEnabled,
		contextCompactTarget:     fm.contextCompactTarget,
		contextCompactMaxRetries: fm.contextCompactMaxRetries,
	}
	newFm.idx.Store(fm.idx.Load())
	for i, m := range fm.models {
		bound, err := m.WithTools(tools)
		if err != nil {
			return nil, fmt.Errorf("bind tools to model %s: %w", fm.names[i], err)
		}
		newFm.models[i] = bound
	}
	return newFm, nil
}

// BindTools binds tools to all underlying models (deprecated interface compat).
func (fm *FailoverChatModel) BindTools(tools []*schema.ToolInfo) error {
	for i, m := range fm.models {
		if cm, ok := m.(model.ChatModel); ok {
			if err := cm.BindTools(tools); err != nil {
				return fmt.Errorf("bind tools to model %s: %w", fm.names[i], err)
			}
		}
	}
	return nil
}
