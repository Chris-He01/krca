package ck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRenderSQL_Strings(t *testing.T) {
	out, err := RenderSQL("SELECT * FROM t WHERE host = ${h}", map[string]any{"h": "k8s-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "'k8s-1'") {
		t.Fatalf("expected quoted string, got %q", out)
	}
}

func TestRenderSQL_QuoteEscape(t *testing.T) {
	out, err := RenderSQL("WHERE name = ${n}", map[string]any{"n": "o'reilly"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "'o''reilly'") {
		t.Fatalf("expected SQL-escaped quote, got %q", out)
	}
}

func TestRenderSQL_TimeBuiltins(t *testing.T) {
	out, err := RenderSQL("WHERE ts >= ${hour_ago} AND ts <= ${now}", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Both should be rendered as ClickHouse datetime literals (quoted).
	if strings.Count(out, "'") < 4 {
		t.Fatalf("expected two quoted timestamps, got %q", out)
	}
	if strings.Contains(out, "${") {
		t.Fatalf("placeholder not substituted: %q", out)
	}
}

func TestRenderSQL_Unresolved(t *testing.T) {
	_, err := RenderSQL("WHERE x = ${missing}", nil)
	if err == nil {
		t.Fatal("expected error for unresolved placeholder")
	}
}

func TestRenderSQL_Types(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"int", map[string]any{"v": 42}, "= 42"},
		{"floatInt", map[string]any{"v": float64(42)}, "= 42"},
		{"float", map[string]any{"v": 1.5}, "= 1.5"},
		{"boolTrue", map[string]any{"v": true}, "= 1"},
		{"boolFalse", map[string]any{"v": false}, "= 0"},
		{"nil", map[string]any{"v": nil}, "= NULL"},
		{"list", map[string]any{"v": []any{1, 2, 3}}, "= 1, 2, 3"},
		{"stringList", map[string]any{"v": []string{"a", "b"}}, "= 'a', 'b'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := RenderSQL("WHERE x = ${v}", c.params)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, c.want) {
				t.Fatalf("want %q in %q", c.want, out)
			}
		})
	}
}

func TestRenderSQL_TimeValue(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	ts := time.Date(2026, 5, 13, 10, 30, 0, 0, loc)
	out, err := RenderSQL("WHERE ts = ${t}", map[string]any{"t": ts})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "'2026-05-13 10:30:00'") {
		t.Fatalf("unexpected time render: %q", out)
	}
}

func TestApplyDefaultFormat(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want string
	}{
		{"appends to plain select", "SELECT 1", "SELECT 1 FORMAT JSON"},
		{"appends after limit", "SELECT * FROM t LIMIT 5", "SELECT * FROM t LIMIT 5 FORMAT JSON"},
		{"trim trailing semi", "SELECT 1;", "SELECT 1 FORMAT JSON"},
		{"respects existing FORMAT", "SELECT 1 FORMAT TabSeparated", "SELECT 1 FORMAT TabSeparated"},
		{"respects FORMAT JSONEachRow", "SELECT 1 FORMAT JSONEachRow", "SELECT 1 FORMAT JSONEachRow"},
		{"non-select unchanged", "INSERT INTO t VALUES (1)", "INSERT INTO t VALUES (1)"},
		{"with CTE", "WITH x AS (SELECT 1) SELECT * FROM x", "WITH x AS (SELECT 1) SELECT * FROM x FORMAT JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyDefaultFormat(tc.sql)
			if got != tc.want {
				t.Fatalf("want %q got %q", tc.want, got)
			}
		})
	}
}

func TestQueryRaw_AutoAppendsFormatJSON(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c, _ := New(Config{GatewayURL: srv.URL, Token: "tok"})
	_, _, _ = c.QueryRaw(context.Background(), "SELECT 1 LIMIT 5", QueryOptions{Limit: -1})
	if !strings.HasSuffix(strings.TrimRight(captured, " \t\r\n"), "FORMAT JSON") {
		t.Fatalf("expected FORMAT JSON appended, got %q", captured)
	}
}

// TestQueryRows_AutoAppendsFormatJSON guards against the QueryRows code
// path skipping the prepare() helper and shipping a TabSeparated request
// to the gateway. node-health goes through this path.
func TestQueryRows_AutoAppendsFormatJSON(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		_, _ = w.Write([]byte(`{"meta":[],"data":[{"x":1}],"rows":1}`))
	}))
	defer srv.Close()

	c, _ := New(Config{GatewayURL: srv.URL, Token: "tok"})
	res, err := c.QueryRows(context.Background(), "SELECT 1", QueryOptions{Limit: -1})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !strings.HasSuffix(strings.TrimRight(captured, " \t\r\n"), "FORMAT JSON") {
		t.Fatalf("expected FORMAT JSON on QueryRows path, got %q", captured)
	}
	if res.RowCount != 1 {
		t.Fatalf("expected to decode 1 row from FORMAT JSON envelope, got %d", res.RowCount)
	}
}

// TestQueryNodeHealth_AutoAppendsFormatJSON guards the typed node-health
// path specifically — it must also produce FORMAT JSON.
func TestQueryNodeHealth_AutoAppendsFormatJSON(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c, _ := New(Config{GatewayURL: srv.URL, Token: "tok"})
	_, _ = c.QueryNodeHealth(context.Background(), NodeHealthRequest{
		Labels:     []string{"node_cpu_sys_unhealth"},
		PDateStart: "20260513",
		PDateEnd:   "20260513",
		Limit:      5,
	})
	if !strings.HasSuffix(strings.TrimRight(captured, " \t\r\n"), "FORMAT JSON") {
		t.Fatalf("expected FORMAT JSON on node-health path, got %q", captured)
	}
}

func TestApplyDefaultLimit(t *testing.T) {
	c := &Client{cfg: Config{DefaultLimit: 1000}}
	cases := []struct {
		name   string
		sql    string
		override int
		want   string
	}{
		{"adds default", "SELECT 1", 0, "SELECT 1 LIMIT 1000"},
		{"respects existing", "SELECT 1 LIMIT 5", 0, "SELECT 1 LIMIT 5"},
		{"override", "SELECT 1", 50, "SELECT 1 LIMIT 50"},
		{"disabled by -1", "SELECT 1", -1, "SELECT 1"},
		{"trailing semicolon", "SELECT 1;", 0, "SELECT 1 LIMIT 1000"},
		{"not a select", "EXPLAIN SELECT 1", 0, "EXPLAIN SELECT 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.applyDefaultLimit(tc.sql, tc.override)
			if got != tc.want {
				t.Fatalf("want %q got %q", tc.want, got)
			}
		})
	}
}

func TestClient_QueryRows_RoundTrip(t *testing.T) {
	var capturedSQL string
	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedSQL = string(body)
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"host":"h1","n":3},{"host":"h2","n":5}]}`))
	}))
	defer srv.Close()

	c, err := New(Config{
		GatewayURL:   srv.URL,
		Token:        "fake-token",
		User:         "tester",
		Principal:    "tester/user@example.com",
		DefaultLimit: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.QueryRows(context.Background(), "SELECT host, count() AS n FROM t WHERE host = ${h} GROUP BY host", QueryOptions{
		Params: map[string]any{"h": "h'1"},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.RowCount != 2 {
		t.Fatalf("want 2 rows got %d", res.RowCount)
	}
	if !strings.Contains(capturedSQL, "'h''1'") {
		t.Fatalf("escaped value missing in body: %q", capturedSQL)
	}
	if capturedHeaders.Get("Ks-Auth-Token") != "fake-token" {
		t.Fatalf("missing Ks-Auth-Token: %#v", capturedHeaders)
	}
	if capturedHeaders.Get("Ks-Auth-Type") != "USER" {
		t.Fatalf("missing Ks-Auth-Type")
	}
	if capturedHeaders.Get("Ks-Auth-User") != "tester" {
		t.Fatalf("missing Ks-Auth-User")
	}
	if capturedHeaders.Get("Ks-Auth-Principal") != "tester/user@example.com" {
		t.Fatalf("missing Ks-Auth-Principal")
	}
	if capturedHeaders.Get("Ks-Query-Id") == "" {
		t.Fatalf("missing Ks-Query-Id")
	}
	if capturedHeaders.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("wrong content-type: %q", capturedHeaders.Get("Content-Type"))
	}
	wantCols := map[string]bool{"host": true, "n": true}
	for _, c := range res.Columns {
		if !wantCols[c] {
			t.Fatalf("unexpected column %q", c)
		}
	}
}

func TestClient_QueryRows_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer srv.Close()

	c, _ := New(Config{GatewayURL: srv.URL, Token: "tok"})
	res, err := c.QueryRows(context.Background(), "SELECT 1", QueryOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if res == nil || len(res.Raw) == 0 {
		t.Fatal("expected raw body to be preserved on error")
	}
	var parsed map[string]any
	if jerr := json.Unmarshal(res.Raw, &parsed); jerr != nil {
		t.Fatalf("raw body not JSON: %v", jerr)
	}
	if parsed["error"] != "bad token" {
		t.Fatalf("unexpected raw: %v", parsed)
	}
}

func TestClient_QueryRows_DefaultLimitApplied(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c, _ := New(Config{GatewayURL: srv.URL, Token: "tok", DefaultLimit: 7})
	_, _ = c.QueryRows(context.Background(), "SELECT * FROM t WHERE host = ${h}", QueryOptions{
		Params: map[string]any{"h": "x"},
	})
	if !strings.Contains(captured, "LIMIT 7") {
		t.Fatalf("default LIMIT not applied: %q", captured)
	}
}
