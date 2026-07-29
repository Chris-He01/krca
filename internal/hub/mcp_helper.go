package hub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mcptool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const defaultChunkMaxChars = 4000

// MCPRateLimiter is a token-bucket rate limiter shared across MCP tool invocations.
type MCPRateLimiter struct {
	ch chan struct{}
}

// NewMCPRateLimiter creates a limiter allowing at most qps calls per second (burst = qps).
func NewMCPRateLimiter(qps float64) *MCPRateLimiter {
	burst := int(qps)
	if burst < 1 {
		burst = 1
	}
	interval := time.Duration(float64(time.Second) / qps)
	rl := &MCPRateLimiter{ch: make(chan struct{}, burst)}
	// Pre-fill
	for i := 0; i < burst; i++ {
		rl.ch <- struct{}{}
	}
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			select {
			case rl.ch <- struct{}{}:
			default:
			}
		}
	}()
	return rl
}

// Wait blocks until a token is available or ctx is cancelled.
func (rl *MCPRateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// rateLimitedTool wraps an InvokableTool and enforces a shared MCPRateLimiter.
type rateLimitedTool struct {
	inner tool.InvokableTool
	rl    *MCPRateLimiter
}

func (t *rateLimitedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.inner.Info(ctx)
}

func (t *rateLimitedTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if err := t.rl.Wait(ctx); err != nil {
		return "", fmt.Errorf("mcp rate limit: %w", err)
	}
	return t.inner.InvokableRun(ctx, argumentsInJSON, opts...)
}

// WrapToolsWithRateLimit wraps each InvokableTool in tools with the given rate limiter.
// The get_result_chunk helper is excluded (it reads from local memory, not remote MCP).
func WrapToolsWithRateLimit(tools []tool.BaseTool, rl *MCPRateLimiter) []tool.BaseTool {
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		info, _ := t.Info(context.Background())
		if invokable, ok := t.(tool.InvokableTool); ok && (info == nil || info.Name != "get_result_chunk") {
			wrapped[i] = &rateLimitedTool{inner: invokable, rl: rl}
		} else {
			wrapped[i] = t
		}
	}
	return wrapped
}

// mcpToolSet holds MCP tools along with a shared chunk retrieval tool.
// It encapsulates large-result chunking so callers don't need to know the details.
type mcpToolSet struct {
	mu       sync.Mutex
	maxChars int
	chunks   map[string][]string // chunkID -> ordered slices
}

func newMCPToolSet(maxChars int) *mcpToolSet {
	if maxChars <= 0 {
		maxChars = defaultChunkMaxChars
	}
	return &mcpToolSet{
		maxChars: maxChars,
		chunks:   make(map[string][]string),
	}
}

// storeChunked splits content into chunks if it exceeds maxChars.
// Returns the first chunk (with retrieval instructions if chunked).
func (s *mcpToolSet) storeChunked(toolName string, content string) string {
	if len(content) <= s.maxChars {
		return content
	}
	var parts []string
	for i := 0; i < len(content); i += s.maxChars {
		end := i + s.maxChars
		if end > len(content) {
			end = len(content)
		}
		parts = append(parts, content[i:end])
	}

	s.mu.Lock()
	id := fmt.Sprintf("%s_%d", toolName, len(s.chunks))
	s.chunks[id] = parts
	s.mu.Unlock()

	header := fmt.Sprintf("[CHUNKED_RESULT: chunk_id=%q, showing_slide=1/%d, total_chars=%d]\n"+
		"调用 get_result_chunk(chunk_id=%q, index=2) 获取下一片段。\n"+
		"建议：每次获取一片后先提取关键信息，再获取下一片。\n\n",
		id, len(parts), len(content), id)
	return header + parts[0]
}

// getChunk returns the chunk at the given 1-based index.
func (s *mcpToolSet) getChunk(chunkID string, index int) (string, error) {
	s.mu.Lock()
	parts, ok := s.chunks[chunkID]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("chunk_id %q not found", chunkID)
	}
	if index < 1 || index > len(parts) {
		return "", fmt.Errorf("index %d out of range [1, %d]", index, len(parts))
	}
	result := fmt.Sprintf("[CHUNK %d/%d of %q]\n\n%s", index, len(parts), chunkID, parts[index-1])
	if index < len(parts) {
		result += fmt.Sprintf("\n\n[还有 %d 片段未读，调用 get_result_chunk(chunk_id=%q, index=%d) 继续]",
			len(parts)-index, chunkID, index+1)
	} else {
		result += "\n\n[所有片段已读取完毕]"
	}
	return result, nil
}

func (s *mcpToolSet) cleanup() {
	s.mu.Lock()
	s.chunks = make(map[string][]string)
	s.mu.Unlock()
}

// chunkRetrievalTool returns a tool that retrieves chunked results by ID and index.
func (s *mcpToolSet) chunkRetrievalTool() tool.InvokableTool {
	return &chunkRetriever{set: s}
}

type chunkRetriever struct {
	set *mcpToolSet
}

func (t *chunkRetriever) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_result_chunk",
		Desc: "获取大结果的下一个片段。当工具返回 [CHUNKED_RESULT] 标记时，使用此工具按滑动窗口方式逐片获取后续数据。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"chunk_id": {Type: schema.String, Desc: "分块结果的 ID", Required: true},
			"index":    {Type: schema.Integer, Desc: "片段序号（从 1 开始）", Required: true},
		}),
	}, nil
}

func (t *chunkRetriever) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		ChunkID string `json:"chunk_id"`
		Index   int    `json:"index"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	return t.set.getChunk(args.ChunkID, args.Index)
}

// ConnectMCP connects to an MCP endpoint and returns all tools (including the
// chunk retrieval tool). Large tool results are automatically chunked so the
// LLM processes them in a sliding-window fashion. MCP isError responses are
// converted to normal text for graceful LLM handling.
//
// The returned cleanup function should be called between runs to free chunk memory.
func ConnectMCP(ctx context.Context, cfg MCPConfig, workspaceDir ...string) (tools []tool.BaseTool, cli *client.Client, cleanup func(), err error) {
	// workspaceDir is used to save image results from MCP tools as files.
	wsDir := "sandbox/workspace"
	if len(workspaceDir) > 0 && workspaceDir[0] != "" {
		wsDir = workspaceDir[0]
	}
	if cfg.Endpoint() == "" {
		return nil, nil, nil, fmt.Errorf("mcp %q: url or sse_url is required", cfg.Name)
	}

	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}

	// Per-user token manager: if sign_token_url is set, tokens are signed per-user
	// and injected dynamically via headerFunc on each request.
	var tokenMgr *TokenManager
	if cfg.SignTokenURL != "" && cfg.APIKey != "" {
		// Token validity: 365 days, cached in memory until expiry
		ttl := 365 * 24 * time.Hour
		tokenMgr = NewTokenManager(cfg.SignTokenURL, cfg.APIKey, ttl)
		log.Printf("mcp %q: per-user token signing enabled (url=%s, ttl=%s)", cfg.Name, cfg.SignTokenURL, ttl)
	}

	// Use a timeout for initial connection to avoid blocking startup forever
	connCtx, connCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connCancel()

	if cfg.IsStreamableHTTP() {
		// Streamable HTTP transport
		var httpOpts []transport.StreamableHTTPCOption
		if len(headers) > 0 {
			httpOpts = append(httpOpts, transport.WithHTTPHeaders(headers))
		}
		// Dynamic per-user header injection
		if tokenMgr != nil {
			httpOpts = append(httpOpts, transport.WithHTTPHeaderFunc(tokenMgr.HeaderFunc(cfg.APIKey)))
		}
		httpTransport, tErr := transport.NewStreamableHTTP(cfg.URL, httpOpts...)
		if tErr != nil {
			return nil, nil, nil, fmt.Errorf("mcp %q: %w", cfg.Name, tErr)
		}
		cli = client.NewClient(httpTransport)
	} else {
		// SSE transport
		var opts []transport.ClientOption
		if len(headers) > 0 {
			opts = append(opts, client.WithHeaders(headers))
		}
		// Dynamic per-user header injection for SSE
		if tokenMgr != nil {
			opts = append(opts, transport.WithHeaderFunc(tokenMgr.HeaderFunc(cfg.APIKey)))
		}
		cli, err = client.NewSSEMCPClient(cfg.SSEURL, opts...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("mcp %q: %w", cfg.Name, err)
		}
	}

	if err = cli.Start(connCtx); err != nil {
		return nil, nil, nil, fmt.Errorf("mcp %q: start: %w", cfg.Name, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "knsight-hub", Version: "0.1.0"}
	if _, err = cli.Initialize(connCtx, initReq); err != nil {
		return nil, nil, nil, fmt.Errorf("mcp %q: initialize: %w", cfg.Name, err)
	}

	ts := newMCPToolSet(defaultChunkMaxChars)

	// Wrap with reconnectable client so tools auto-reconnect on session expiry
	rcli := newReconnectableClient(cli, cfg, tokenMgr)

	mcpTools, err := mcptool.GetTools(ctx, &mcptool.Config{
		Cli: rcli,
		ToolCallResultHandler: func(_ context.Context, name string, result *mcp.CallToolResult) (*mcp.CallToolResult, error) {
			// Convert isError to normal text for graceful LLM handling.
			if result.IsError {
				result.IsError = false
			}

			// Process each content item: chunk text, convert images to descriptions.
			var processed []mcp.Content
			for _, c := range result.Content {
				switch item := c.(type) {
				case mcp.ImageContent:
					// Save base64 image to workspace file, replace content with path reference.
					// This avoids sending ~50K chars of base64 per image to the LLM.
					ext := ".png"
					if strings.Contains(item.MIMEType, "jpeg") || strings.Contains(item.MIMEType, "jpg") {
						ext = ".jpg"
					} else if strings.Contains(item.MIMEType, "svg") {
						ext = ".svg"
					}
					imgName := fmt.Sprintf("%s_%d%s", name, time.Now().UnixMilli(), ext)
					imgPath := filepath.Join(wsDir, imgName)
					imgData, decErr := base64.StdEncoding.DecodeString(item.Data)
					if decErr != nil {
						imgData, decErr = base64.URLEncoding.DecodeString(item.Data)
					}
					if decErr == nil {
						if mkErr := os.MkdirAll(wsDir, 0755); mkErr == nil {
							if wErr := os.WriteFile(imgPath, imgData, 0644); wErr == nil {
								log.Printf("mcp %q: saved image (%d bytes) to %s", name, len(imgData), imgPath)
							}
						}
					}
					desc := fmt.Sprintf("[IMAGE saved: path=%s, mime=%s, size=%d bytes — 图表已保存，可通过 read_file 或前端查看]",
						imgPath, item.MIMEType, len(item.Data)*3/4)
					processed = append(processed, mcp.NewTextContent(desc))
				case mcp.TextContent:
					chunked := ts.storeChunked(name, item.Text)
					if chunked != item.Text {
						processed = append(processed, mcp.NewTextContent(chunked))
					} else {
						processed = append(processed, item)
					}
				default:
					processed = append(processed, c)
				}
			}
			result.Content = processed
			return result, nil
		},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("mcp %q: get tools: %w", cfg.Name, err)
	}

	// Append chunk retrieval tool
	tools = append(mcpTools, ts.chunkRetrievalTool())
	return tools, cli, ts.cleanup, nil
}
