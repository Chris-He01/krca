package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// reconnectableClient wraps a client.MCPClient and auto-reconnects
// when the MCP server returns "Invalid session ID" (session expired).
type reconnectableClient struct {
	mu       sync.Mutex
	inner    *client.Client
	cfg      MCPConfig
	tokenMgr *TokenManager // may be nil
}

func newReconnectableClient(cli *client.Client, cfg MCPConfig, tokenMgr *TokenManager) *reconnectableClient {
	return &reconnectableClient{
		inner:    cli,
		cfg:      cfg,
		tokenMgr: tokenMgr,
	}
}

func (r *reconnectableClient) reconnect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close old client (ignore errors — it may already be dead)
	_ = r.inner.Close()

	headers := map[string]string{}
	if r.cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + r.cfg.APIKey
	}

	var cli *client.Client
	var err error

	if r.cfg.IsStreamableHTTP() {
		var httpOpts []transport.StreamableHTTPCOption
		if len(headers) > 0 {
			httpOpts = append(httpOpts, transport.WithHTTPHeaders(headers))
		}
		if r.tokenMgr != nil {
			httpOpts = append(httpOpts, transport.WithHTTPHeaderFunc(r.tokenMgr.HeaderFunc(r.cfg.APIKey)))
		}
		httpTransport, tErr := transport.NewStreamableHTTP(r.cfg.URL, httpOpts...)
		if tErr != nil {
			return fmt.Errorf("mcp %q reconnect: new transport: %w", r.cfg.Name, tErr)
		}
		cli = client.NewClient(httpTransport)
	} else {
		var opts []transport.ClientOption
		if len(headers) > 0 {
			opts = append(opts, client.WithHeaders(headers))
		}
		if r.tokenMgr != nil {
			opts = append(opts, transport.WithHeaderFunc(r.tokenMgr.HeaderFunc(r.cfg.APIKey)))
		}
		cli, err = client.NewSSEMCPClient(r.cfg.SSEURL, opts...)
		if err != nil {
			return fmt.Errorf("mcp %q reconnect: new client: %w", r.cfg.Name, err)
		}
	}

	if err = cli.Start(ctx); err != nil {
		return fmt.Errorf("mcp %q reconnect: start: %w", r.cfg.Name, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "knsight-hub", Version: "0.1.0"}
	if _, err = cli.Initialize(ctx, initReq); err != nil {
		_ = cli.Close()
		return fmt.Errorf("mcp %q reconnect: initialize: %w", r.cfg.Name, err)
	}

	r.inner = cli
	log.Printf("mcp %q: reconnected to %s", r.cfg.Name, r.cfg.Endpoint())
	return nil
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation/deadline is never retryable — the caller has gone away.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Invalid session ID") ||
		(strings.Contains(s, "session") && strings.Contains(s, "expired")) ||
		strings.Contains(s, "unexpected EOF") ||
		strings.Contains(s, "status 502") ||
		strings.Contains(s, "status 503") ||
		strings.Contains(s, "status 504") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset")
}

// CallTool with auto-reconnect on session error.
// Retries up to 6 times with 10s delay (total ~1min), then returns error as tool output
// so the agent can adapt instead of the whole session failing.
func (r *reconnectableClient) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := request.Params.Name
	argsJSON := ""
	if request.Params.Arguments != nil {
		if b, jErr := json.Marshal(request.Params.Arguments); jErr == nil {
			argsJSON = string(b)
			if len(argsJSON) > 500 {
				argsJSON = argsJSON[:497] + "..."
			}
		}
	}

	result, err := r.inner.CallTool(ctx, request)
	if err == nil {
		return result, nil
	}
	if !isRetryableError(err) {
		// Non-retryable error — return as tool result so agent can continue
		log.Printf("mcp %q tool %s: non-retryable error (returning to agent): %v | args: %s", r.cfg.Name, toolName, err, argsJSON)
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: fmt.Sprintf("工具 %s 调用失败: %v\n请尝试其他方式或跳过此步骤。", toolName, err)}},
			IsError: true,
		}, nil
	}

	// Check if parent context is already done — return error as result
	if ctx.Err() != nil {
		log.Printf("mcp %q tool %s: context cancelled, returning to agent: %v | args: %s", r.cfg.Name, toolName, err, argsJSON)
		NotifyError("mcp:ctx:"+r.cfg.Name+":"+toolName, ErrorParams{
			Component: "MCP",
			Tool:      r.cfg.Name + "/" + toolName,
			Input:     argsJSON,
			Error:     fmt.Sprintf("context cancelled: %v", err),
		})
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: fmt.Sprintf("工具 %s 调用被取消（超时或连接中断）。\n请基于已有数据继续分析。", toolName)}},
			IsError: true,
		}, nil
	}

	origErr := err
	const maxRetries = 3
	const retryDelay = 3 * time.Second

	for i := 1; i <= maxRetries; i++ {
		log.Printf("mcp %q tool %s: error, reconnecting (%d/%d) after %s | args: %s | error: %v",
			r.cfg.Name, toolName, i, maxRetries, retryDelay, argsJSON, origErr)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("mcp %q: context cancelled during reconnect: %w", r.cfg.Name, ctx.Err())
		case <-time.After(retryDelay):
		}

		if reconnErr := r.reconnect(ctx); reconnErr != nil {
			log.Printf("mcp %q: reconnect attempt %d failed: %v", r.cfg.Name, i, reconnErr)
			continue
		}

		result, err = r.inner.CallTool(ctx, request)
		if err == nil || !isRetryableError(err) {
			if err == nil {
				log.Printf("mcp %q tool %s: retry %d succeeded", r.cfg.Name, toolName, i)
			}
			return result, err
		}
		log.Printf("mcp %q tool %s: retry %d failed: %v | args: %s", r.cfg.Name, toolName, i, err, argsJSON)
	}

	// All retries exhausted — notify and return error as tool result
	log.Printf("mcp %q: all %d reconnect attempts failed, returning error to agent | args: %s", r.cfg.Name, maxRetries, argsJSON)

	NotifyError("mcp:"+r.cfg.Name+":"+toolName, ErrorParams{
		Component: "MCP",
		Tool:      r.cfg.Name + "/" + toolName,
		Input:     argsJSON,
		Error:     origErr.Error(),
		Retries:   maxRetries,
	})

	// Return error as tool result — agent can see and adapt
	errText := fmt.Sprintf("工具 %s 调用失败（重试 %d 次后仍失败）。\n错误: %v\n\n请尝试：\n1. 简化脚本（减少数据量，如 head -50 代替 head -100）\n2. 拆分为多个小脚本分步执行\n3. 跳过此步骤，基于已有数据继续分析",
		toolName, maxRetries, origErr)
	errResult := &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: errText,
		}},
		IsError: true,
	}
	return errResult, nil
}

// --- Delegate all other MCPClient methods to inner ---

func (r *reconnectableClient) Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	return r.inner.Initialize(ctx, request)
}

func (r *reconnectableClient) Ping(ctx context.Context) error {
	return r.inner.Ping(ctx)
}

func (r *reconnectableClient) ListResourcesByPage(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return r.inner.ListResourcesByPage(ctx, request)
}

func (r *reconnectableClient) ListResources(ctx context.Context, request mcp.ListResourcesRequest) (*mcp.ListResourcesResult, error) {
	return r.inner.ListResources(ctx, request)
}

func (r *reconnectableClient) ListResourceTemplatesByPage(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return r.inner.ListResourceTemplatesByPage(ctx, request)
}

func (r *reconnectableClient) ListResourceTemplates(ctx context.Context, request mcp.ListResourceTemplatesRequest) (*mcp.ListResourceTemplatesResult, error) {
	return r.inner.ListResourceTemplates(ctx, request)
}

func (r *reconnectableClient) ReadResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return r.inner.ReadResource(ctx, request)
}

func (r *reconnectableClient) Subscribe(ctx context.Context, request mcp.SubscribeRequest) error {
	return r.inner.Subscribe(ctx, request)
}

func (r *reconnectableClient) Unsubscribe(ctx context.Context, request mcp.UnsubscribeRequest) error {
	return r.inner.Unsubscribe(ctx, request)
}

func (r *reconnectableClient) ListPromptsByPage(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return r.inner.ListPromptsByPage(ctx, request)
}

func (r *reconnectableClient) ListPrompts(ctx context.Context, request mcp.ListPromptsRequest) (*mcp.ListPromptsResult, error) {
	return r.inner.ListPrompts(ctx, request)
}

func (r *reconnectableClient) GetPrompt(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return r.inner.GetPrompt(ctx, request)
}

func (r *reconnectableClient) ListToolsByPage(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return r.inner.ListToolsByPage(ctx, request)
}

func (r *reconnectableClient) ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	result, err := r.inner.ListTools(ctx, request)
	if err != nil && isRetryableError(err) {
		if reconnErr := r.reconnect(ctx); reconnErr != nil {
			return nil, fmt.Errorf("%w (reconnect also failed: %v)", err, reconnErr)
		}
		return r.inner.ListTools(ctx, request)
	}
	return result, err
}

func (r *reconnectableClient) SetLevel(ctx context.Context, request mcp.SetLevelRequest) error {
	return r.inner.SetLevel(ctx, request)
}

func (r *reconnectableClient) Complete(ctx context.Context, request mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return r.inner.Complete(ctx, request)
}

func (r *reconnectableClient) Close() error {
	return r.inner.Close()
}

func (r *reconnectableClient) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	r.inner.OnNotification(handler)
}
