// Package ck is a small HTTP client for querying ClickHouse through the
// internal themis-olap-gateway proxy. The wire protocol mirrors the
// reference implementation used by sysobservable-alert-manager:
//
//   - POST <gateway>/ with raw SQL in the body and Content-Type
//     application/x-www-form-urlencoded.
//   - Auth via Ks-Auth-* headers (Token / Type / User / Principal).
//   - A per-call Ks-Query-Id (UUIDv4) for tracing.
//
// On top of that the client adds a few ergonomics the LLM-side skill needs:
// parameter binding with safe quoting (esp. for time ranges), an optional
// default LIMIT, and a structured response that exposes rows + columns when
// the gateway returns JSON.
package ck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config holds connection settings for the ClickHouse gateway.
type Config struct {
	// GatewayURL is the full base URL of the OLAP gateway, e.g.
	// "https://analytics.example.com".
	GatewayURL string

	// Token is the Ks-Auth-Token bearer. Required.
	Token string

	// User is the Ks-Auth-User principal short name, e.g. "liusi".
	User string

	// Principal is the Ks-Auth-Principal full identifier, e.g.
	// "service-account".
	Principal string

	// AuthType defaults to "USER" when empty.
	AuthType string

	// DefaultTimeout is applied when a request supplies none. Defaults to
	// 300 seconds, matching the reference implementation.
	DefaultTimeout time.Duration

	// DefaultLimit, when > 0, is appended as "LIMIT N" to SELECT statements
	// that don't already contain a LIMIT clause. Set to 0 to disable.
	DefaultLimit int

	// MaxRowsReturned bounds the row count copied into the structured
	// response. The raw body is always preserved. 0 means unlimited.
	MaxRowsReturned int

	// HTTPClient is optional; if nil a default is constructed with the
	// configured timeout.
	HTTPClient *http.Client
}

// Client is the gateway proxy client.
type Client struct {
	cfg  Config
	http *http.Client
}

// New constructs a Client. It returns an error when required fields are
// missing so that callers fail fast on startup instead of at first query.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.GatewayURL) == "" {
		return nil, errors.New("ck: gateway_url is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("ck: token is required")
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 300 * time.Second
	}
	if cfg.AuthType == "" {
		cfg.AuthType = "USER"
	}
	if cfg.User == "" {
		// Best-effort default: derive from the principal short name.
		if i := strings.IndexAny(cfg.Principal, "/@"); i > 0 {
			cfg.User = cfg.Principal[:i]
		}
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.DefaultTimeout}
	}
	return &Client{cfg: cfg, http: hc}, nil
}

// QueryOptions tweaks a single query.
type QueryOptions struct {
	// Params is substituted into ${name} placeholders in the SQL. Values
	// are quoted according to type; see RenderSQL for details.
	Params map[string]any

	// Limit overrides Client.DefaultLimit for this call. Negative means
	// "no automatic LIMIT".
	Limit int

	// Timeout overrides Client.DefaultTimeout. Zero falls back to default.
	Timeout time.Duration

	// QueryID overrides the auto-generated Ks-Query-Id. Leave empty in
	// normal use; supply one only when bridging an external trace.
	QueryID string
}

// Result is the structured response from QueryRows. Raw always carries the
// gateway's response body verbatim so callers can fall back to bespoke
// parsing when the schema is non-standard.
type Result struct {
	QueryID     string           `json:"query_id"`
	SQL         string           `json:"sql"`
	ElapsedMs   int64            `json:"elapsed_ms"`
	RowCount    int              `json:"row_count"`
	Columns     []string         `json:"columns,omitempty"`
	Rows        []map[string]any `json:"rows,omitempty"`
	Data        json.RawMessage  `json:"data,omitempty"`
	Raw         json.RawMessage  `json:"raw,omitempty"`
	Truncated   bool             `json:"truncated,omitempty"`
	GatewayHTTP int              `json:"gateway_http_status,omitempty"`
}

// prepare renders placeholders, attaches the default LIMIT, and forces the
// CH JSON envelope so downstream callers can rely on a single response
// shape. Every public entry point (QueryRaw / QueryRows / QueryNodeHealth)
// MUST funnel through here — otherwise TabSeparated leaks back and the
// JSON-based decoders see an empty result set.
func (c *Client) prepare(sql string, opts QueryOptions) (string, error) {
	rendered, err := RenderSQL(sql, opts.Params)
	if err != nil {
		return "", err
	}
	rendered = c.applyDefaultLimit(rendered, opts.Limit)
	rendered = applyDefaultFormat(rendered)
	return rendered, nil
}

// QueryRaw POSTs the rendered SQL to the gateway and returns the raw body.
// Use this when you need control over the response shape.
//
// The gateway proxies ClickHouse, whose default output format is
// TabSeparated. prepare() appends "FORMAT JSON" when the SQL does not
// specify a format — that turns the response into the
// {"meta": [...], "data": [...], "rows": N, ...} envelope our decoders
// understand.
func (c *Client) QueryRaw(ctx context.Context, sql string, opts QueryOptions) ([]byte, string, error) {
	rendered, err := c.prepare(sql, opts)
	if err != nil {
		return nil, "", err
	}
	return c.execute(ctx, rendered, opts)
}

// execute does the actual POST. The SQL must already be fully rendered.
func (c *Client) execute(ctx context.Context, rendered string, opts QueryOptions) ([]byte, string, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = c.cfg.DefaultTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	qid := opts.QueryID
	if qid == "" {
		qid = uuid.NewString()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.cfg.GatewayURL, bytes.NewBufferString(rendered))
	if err != nil {
		return nil, qid, fmt.Errorf("ck: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Ks-Auth-Token", c.cfg.Token)
	req.Header.Set("Ks-Auth-Type", c.cfg.AuthType)
	if c.cfg.User != "" {
		req.Header.Set("Ks-Auth-User", c.cfg.User)
	}
	if c.cfg.Principal != "" {
		req.Header.Set("Ks-Auth-Principal", c.cfg.Principal)
	}
	req.Header.Set("Ks-Query-Id", qid)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, qid, fmt.Errorf("ck: gateway request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, qid, fmt.Errorf("ck: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, qid, fmt.Errorf("ck: gateway returned status %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	return body, qid, nil
}

// QueryRows runs the SQL and returns a structured Result. When the gateway
// returns a JSON envelope of the form {"data": <rows>}, the rows slice and
// column order are extracted automatically. Otherwise Data and Raw are
// populated and callers parse them themselves.
func (c *Client) QueryRows(ctx context.Context, sql string, opts QueryOptions) (*Result, error) {
	rendered, err := c.prepare(sql, opts)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	body, qid, err := c.execute(ctx, rendered, opts)
	elapsed := time.Since(start).Milliseconds()
	result := &Result{
		QueryID:   qid,
		SQL:       rendered,
		ElapsedMs: elapsed,
	}
	if err != nil {
		if len(body) > 0 {
			result.Raw = json.RawMessage(body)
		}
		return result, err
	}
	if len(body) == 0 {
		return result, nil
	}
	result.Raw = json.RawMessage(body)

	// The gateway response shape is `{"data": ...}`. Pull data out when
	// possible; otherwise leave Raw set and let the caller decide.
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if jerr := json.Unmarshal(body, &env); jerr == nil && len(env.Data) > 0 {
		result.Data = env.Data
		c.populateRows(result, env.Data)
	} else {
		// No "data" wrapper — best effort: maybe the body is already an
		// array of objects.
		c.populateRows(result, body)
	}
	return result, nil
}

func (c *Client) populateRows(result *Result, data json.RawMessage) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return
	}
	var rows []map[string]any
	if err := json.Unmarshal(trimmed, &rows); err != nil {
		return
	}
	result.RowCount = len(rows)
	if c.cfg.MaxRowsReturned > 0 && len(rows) > c.cfg.MaxRowsReturned {
		rows = rows[:c.cfg.MaxRowsReturned]
		result.Truncated = true
	}
	result.Rows = rows
	if len(rows) > 0 {
		result.Columns = orderedKeys(rows[0])
	}
}

func (c *Client) applyDefaultLimit(sql string, override int) string {
	if override < 0 {
		return sql
	}
	limit := override
	if limit == 0 {
		limit = c.cfg.DefaultLimit
	}
	if limit <= 0 {
		return sql
	}
	if !isSelect(sql) {
		return sql
	}
	if hasLimit(sql) {
		return sql
	}
	trimmed := strings.TrimRight(sql, "; \t\r\n")
	return fmt.Sprintf("%s LIMIT %d", trimmed, limit)
}

var (
	reSelect = regexp.MustCompile(`(?is)^\s*(with\b.*?\bselect\b|select\b)`)
	reLimit  = regexp.MustCompile(`(?is)\blimit\b\s+\d+(\s*,\s*\d+)?\s*$|\blimit\b\s+\d+\s+by\b|\blimit\b\s+\d+\s*$`)
	reFormat = regexp.MustCompile(`(?is)\bformat\s+[A-Za-z][A-Za-z0-9_]*\s*;?\s*$`)
)

func isSelect(sql string) bool { return reSelect.MatchString(sql) }
func hasLimit(sql string) bool {
	trimmed := strings.TrimRight(sql, "; \t\r\n")
	return reLimit.MatchString(trimmed)
}
func hasFormat(sql string) bool { return reFormat.MatchString(sql) }

// applyDefaultFormat appends "FORMAT JSON" to a SELECT-shaped query when no
// FORMAT clause is present. Non-SELECT queries are passed through unchanged
// — DDL/INSERT have undefined behaviour against the gateway anyway and
// FORMAT JSON only makes sense for result-returning statements.
func applyDefaultFormat(sql string) string {
	if !isSelect(sql) {
		return sql
	}
	if hasFormat(sql) {
		return sql
	}
	trimmed := strings.TrimRight(sql, "; \t\r\n")
	return trimmed + " FORMAT JSON"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func orderedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Stable but not alphabetical: alphabetic gives a deterministic order
	// without leaking JSON map iteration ordering. Callers that care about
	// SELECT-list order should parse Raw themselves.
	sortStrings(keys)
	return keys
}

// sortStrings is a tiny in-place sort to avoid pulling in "sort" purely
// for one slice.
func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
