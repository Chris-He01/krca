package ck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// NodeHealthTable is the ClickHouse table that holds per-node health
// scoring labels. Each row is one (host, label_name, p_hourmin) bucket
// with the structured detail packed into the label_value JSON column.
const NodeHealthTable = "service_portrait_ssd.node_label"

// KnownUnhealthLabels lists the label_name values produced by the upstream
// scoring pipeline. Used both for input validation hints and as the
// default filter when callers ask for "any unhealthy label".
var KnownUnhealthLabels = []string{
	"node_cpu_sys_unhealth",
	"x60_node_data_disk_used_unhealth",
	"x60_node_memfree_unhealth",
	"x60_node_root_disk_used_unhealth",
	"node_health_check",
	"node_ipc_unhealth",
	"node_availability_unhealth",
	"x60_node_load_unhealth",
}

// NodeHealthRequest controls a single node-health query. All filters are
// optional; sane defaults are applied (object=node, p_date covers today
// CST, limit=200, no host/label restriction).
type NodeHealthRequest struct {
	// Hosts is the list of object_id values to match (typically full
	// node hostnames). Empty means "any host".
	Hosts []string `json:"hosts,omitempty"`

	// Labels filters on label_name. Empty means "any label".
	// Special value "unhealth" expands to KnownUnhealthLabels.
	Labels []string `json:"labels,omitempty"`

	// Object filters on the object column. Defaults to "node".
	Object string `json:"object,omitempty"`

	// PDateStart / PDateEnd bound the p_date partition, format YYYYMMDD.
	// If both are empty, falls back to today (CST).
	PDateStart string `json:"p_date_start,omitempty"`
	PDateEnd   string `json:"p_date_end,omitempty"`

	// Since / Until accept "YYYY-MM-DD HH:MM:SS" or RFC3339 and override
	// the p_date range plus add a timestamp predicate. Useful when the
	// caller thinks in incident windows, not in partition keys.
	Since string `json:"since,omitempty"`
	Until string `json:"until,omitempty"`

	// HourminStart / HourminEnd are 4-char "HHMM" bounds on p_hourmin.
	// Both required to take effect.
	HourminStart string `json:"hourmin_start,omitempty"`
	HourminEnd   string `json:"hourmin_end,omitempty"`

	// HealthGrade filters on JSONExtractString(label_value,'health_grade').
	// E.g. ["C","D"] to focus on degraded grades.
	HealthGrade []string `json:"health_grade,omitempty"`

	// IsCloudnative filters the is_cloudnative column. nil = any.
	IsCloudnative *uint8 `json:"is_cloudnative,omitempty"`

	// Limit caps the number of returned rows; default 200. Negative
	// disables the auto-LIMIT (be careful).
	Limit int `json:"limit,omitempty"`

	// IncludeRaw, when true, also returns the gateway's raw response in
	// the response envelope. Defaults to false to keep payloads small.
	IncludeRaw bool `json:"include_raw,omitempty"`
}

// NodeHealthRow is one row, with label_value parsed out into typed fields.
// The original JSON is preserved in LabelValueRaw for completeness.
type NodeHealthRow struct {
	PDate         string          `json:"p_date"`
	PHourmin      string          `json:"p_hourmin"`
	Object        string          `json:"object"`
	ObjectID      string          `json:"object_id"`
	LabelName     string          `json:"label_name"`
	IsCloudnative uint8           `json:"is_cloudnative"`
	Timestamp     int64           `json:"timestamp"`
	FormatTime    string          `json:"format_time,omitempty"`
	HealthGrade   string          `json:"health_grade,omitempty"`
	Exception     string          `json:"exception,omitempty"`
	ServiceCount  int             `json:"service_count,omitempty"`
	ServiceList   []string        `json:"service_list,omitempty"`
	LabelValueRaw json.RawMessage `json:"label_value_raw,omitempty"`
}

// NodeHealthResult wraps the parsed rows plus the SQL actually run, the
// query id (for tracing), and an optional raw gateway body.
type NodeHealthResult struct {
	QueryID   string           `json:"query_id"`
	SQL       string           `json:"sql"`
	ElapsedMs int64            `json:"elapsed_ms"`
	RowCount  int              `json:"row_count"`
	Rows      []NodeHealthRow  `json:"rows"`
	Truncated bool             `json:"truncated,omitempty"`
	Raw       json.RawMessage  `json:"raw,omitempty"`
}

// BuildNodeHealthSQL renders the SQL that QueryNodeHealth executes. It is
// exposed so the handler can echo the SQL back even when the client only
// supplied structured params.
func BuildNodeHealthSQL(req NodeHealthRequest) (string, error) {
	object := req.Object
	if object == "" {
		object = "node"
	}

	// Resolve the time / partition window.
	loc := cstLoc()
	now := time.Now().In(loc)
	pStart := req.PDateStart
	pEnd := req.PDateEnd

	var sinceTS, untilTS int64
	if req.Since != "" {
		t, err := parseFlexTime(req.Since, loc)
		if err != nil {
			return "", fmt.Errorf("since: %w", err)
		}
		sinceTS = t.Unix()
		if pStart == "" {
			pStart = t.Format("20060102")
		}
	}
	if req.Until != "" {
		t, err := parseFlexTime(req.Until, loc)
		if err != nil {
			return "", fmt.Errorf("until: %w", err)
		}
		untilTS = t.Unix()
		if pEnd == "" {
			pEnd = t.Format("20060102")
		}
	}
	if pStart == "" {
		pStart = now.Format("20060102")
	}
	if pEnd == "" {
		pEnd = pStart
	}
	if !isPDate(pStart) || !isPDate(pEnd) {
		return "", fmt.Errorf("p_date must be YYYYMMDD (got start=%q end=%q)", pStart, pEnd)
	}

	// Expand the special "unhealth" alias.
	labels := req.Labels
	for i, l := range labels {
		if strings.EqualFold(l, "unhealth") {
			labels = append(labels[:i], append(append([]string{}, KnownUnhealthLabels...), labels[i+1:]...)...)
			break
		}
	}

	limit := req.Limit
	if limit == 0 {
		limit = 200
	}

	var b strings.Builder
	b.WriteString("SELECT p_date, p_hourmin, object, object_id, label_name, label_value, is_cloudnative, timestamp\n")
	b.WriteString("FROM ")
	b.WriteString(NodeHealthTable)
	b.WriteString("\nWHERE p_date >= ")
	b.WriteString(QuoteString(pStart))
	b.WriteString(" AND p_date <= ")
	b.WriteString(QuoteString(pEnd))
	b.WriteString(" AND object = ")
	b.WriteString(QuoteString(object))
	if len(req.Hosts) > 0 {
		b.WriteString(" AND object_id IN (")
		b.WriteString(QuoteStringList(req.Hosts))
		b.WriteString(")")
	}
	if len(labels) > 0 {
		b.WriteString(" AND label_name IN (")
		b.WriteString(QuoteStringList(labels))
		b.WriteString(")")
	}
	if req.HourminStart != "" && req.HourminEnd != "" {
		if !isHHMM(req.HourminStart) || !isHHMM(req.HourminEnd) {
			return "", fmt.Errorf("hourmin must be 4 digits HHMM (got start=%q end=%q)", req.HourminStart, req.HourminEnd)
		}
		b.WriteString(" AND p_hourmin >= ")
		b.WriteString(QuoteString(req.HourminStart))
		b.WriteString(" AND p_hourmin <= ")
		b.WriteString(QuoteString(req.HourminEnd))
	}
	if sinceTS > 0 {
		b.WriteString(" AND timestamp >= ")
		b.WriteString(strconv.FormatInt(sinceTS, 10))
	}
	if untilTS > 0 {
		b.WriteString(" AND timestamp <= ")
		b.WriteString(strconv.FormatInt(untilTS, 10))
	}
	if len(req.HealthGrade) > 0 {
		b.WriteString(" AND JSONExtractString(label_value, 'health_grade') IN (")
		b.WriteString(QuoteStringList(req.HealthGrade))
		b.WriteString(")")
	}
	if req.IsCloudnative != nil {
		b.WriteString(" AND is_cloudnative = ")
		b.WriteString(strconv.FormatUint(uint64(*req.IsCloudnative), 10))
	}
	b.WriteString(" ORDER BY timestamp DESC, object_id, label_name")
	if limit > 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(strconv.Itoa(limit))
	}
	return b.String(), nil
}

// QueryNodeHealth runs BuildNodeHealthSQL via the underlying client and
// decodes the label_value JSON into typed fields. The raw gateway body is
// preserved on the result when req.IncludeRaw is true or when the parse
// fails partway through.
func (c *Client) QueryNodeHealth(ctx context.Context, req NodeHealthRequest) (*NodeHealthResult, error) {
	sql, err := BuildNodeHealthSQL(req)
	if err != nil {
		return nil, err
	}
	// Disable the default LIMIT — we already produced an explicit one.
	res, err := c.QueryRows(ctx, sql, QueryOptions{Limit: -1})
	out := &NodeHealthResult{SQL: sql}
	if res != nil {
		out.QueryID = res.QueryID
		out.ElapsedMs = res.ElapsedMs
		if req.IncludeRaw {
			out.Raw = res.Raw
		}
	}
	if err != nil {
		if out.Raw == nil && res != nil {
			out.Raw = res.Raw
		}
		return out, err
	}
	out.Rows = make([]NodeHealthRow, 0, len(res.Rows))
	for _, r := range res.Rows {
		out.Rows = append(out.Rows, decodeNodeHealthRow(r))
	}
	out.RowCount = len(out.Rows)
	out.Truncated = res.Truncated
	return out, nil
}

func decodeNodeHealthRow(r map[string]any) NodeHealthRow {
	row := NodeHealthRow{
		PDate:     asString(r["p_date"]),
		PHourmin:  asString(r["p_hourmin"]),
		Object:    asString(r["object"]),
		ObjectID:  asString(r["object_id"]),
		LabelName: asString(r["label_name"]),
		Timestamp: asInt64(r["timestamp"]),
	}
	if v, ok := r["is_cloudnative"]; ok {
		row.IsCloudnative = uint8(asInt64(v))
	}
	// label_value comes back as a string holding JSON. Parse it best-effort.
	if lv, ok := r["label_value"]; ok {
		lvStr := asString(lv)
		if lvStr != "" {
			row.LabelValueRaw = json.RawMessage(lvStr)
			var inner struct {
				Timestamp    int64  `json:"timestamp"`
				FormatTime   string `json:"format_time"`
				HealthGrade  string `json:"health_grade"`
				Exception    string `json:"exception"`
				ServiceCount int    `json:"service_count"`
				ServiceList  any    `json:"service_list"` // may be string-encoded JSON or array
			}
			if err := json.Unmarshal([]byte(lvStr), &inner); err == nil {
				row.FormatTime = inner.FormatTime
				row.HealthGrade = inner.HealthGrade
				row.Exception = inner.Exception
				row.ServiceCount = inner.ServiceCount
				if inner.Timestamp > 0 && row.Timestamp == 0 {
					row.Timestamp = inner.Timestamp
				}
				row.ServiceList = decodeServiceList(inner.ServiceList)
			}
		}
	}
	return row
}

func decodeServiceList(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, asString(item))
		}
		return out
	case string:
		// Upstream stores it as a JSON string e.g. `"[\"svc-a\",\"svc-b\"]"`.
		if x == "" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(x), &arr); err == nil {
			return arr
		}
		return []string{x}
	}
	return nil
}

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}

func isPDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHHMM(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseFlexTime(s string, loc *time.Location) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q (try YYYY-MM-DD HH:MM:SS or RFC3339)", s)
}

// nodeHealthHandler is the HTTP entry point.
func (h *Handler) handleNodeHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONErr(w, http.StatusMethodNotAllowed, "method must be POST")
		return
	}
	if !h.enabled || h.client == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "ck endpoint disabled — set ck.enabled=true and provide a token")
		return
	}
	var req NodeHealthRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	result, err := h.client.QueryNodeHealth(r.Context(), req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     false,
			"error":  err.Error(),
			"result": result,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"ok": true, "result": result})
}
