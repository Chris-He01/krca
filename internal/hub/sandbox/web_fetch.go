package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type WebFetchTool struct {
	timeoutSec   int
	maxBodyBytes int
}

func NewWebFetchTool(timeoutSec, maxBodyBytes int) *WebFetchTool {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 500_000
	}
	return &WebFetchTool{
		timeoutSec:   timeoutSec,
		maxBodyBytes: maxBodyBytes,
	}
}

func (t *WebFetchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_fetch",
		Desc: "Fetch content from a URL. Returns JSON with url, status, length, and text fields.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {Type: schema.String, Desc: "URL to fetch", Required: true},
		}),
	}, nil
}

type webFetchResult struct {
	URL       string `json:"url"`
	FinalURL  string `json:"final_url"`
	Status    int    `json:"status"`
	Truncated bool   `json:"truncated"`
	Length    int    `json:"length"`
	Text      string `json:"text"`
}

func (t *WebFetchTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if args.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(args.URL)
	if err != nil {
		return fmt.Sprintf("error: invalid url: %s", err.Error()), nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "error: only http/https URLs are allowed", nil
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") {
		return "error: fetching local/private addresses is not allowed", nil
	}

	client := &http.Client{
		Timeout: time.Duration(t.timeoutSec) * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return fmt.Sprintf("error: create request: %s", err.Error()), nil
	}
	req.Header.Set("User-Agent", "knsight-agent/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("error: fetch failed: %s", err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(t.maxBodyBytes)+1))
	if err != nil {
		return fmt.Sprintf("error: read body: %s", err.Error()), nil
	}

	truncated := len(body) > t.maxBodyBytes
	if truncated {
		body = body[:t.maxBodyBytes]
	}

	result := webFetchResult{
		URL:       args.URL,
		FinalURL:  resp.Request.URL.String(),
		Status:    resp.StatusCode,
		Truncated: truncated,
		Length:    len(body),
		Text:      string(body),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
