package ck

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildNodeHealthSQL_Defaults(t *testing.T) {
	sql, err := BuildNodeHealthSQL(NodeHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM " + NodeHealthTable,
		"object = 'node'",
		"ORDER BY timestamp DESC",
		"LIMIT 200",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestBuildNodeHealthSQL_Filters(t *testing.T) {
	one := uint8(1)
	sql, err := BuildNodeHealthSQL(NodeHealthRequest{
		Hosts:         []string{"host'1", "host2"},
		Labels:        []string{"node_cpu_sys_unhealth"},
		PDateStart:    "20260501",
		PDateEnd:      "20260513",
		HourminStart:  "0000",
		HourminEnd:    "0830",
		HealthGrade:   []string{"C", "D"},
		IsCloudnative: &one,
		Limit:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"p_date >= '20260501'",
		"p_date <= '20260513'",
		"object_id IN ('host''1', 'host2')",
		"label_name IN ('node_cpu_sys_unhealth')",
		"p_hourmin >= '0000'",
		"p_hourmin <= '0830'",
		"JSONExtractString(label_value, 'health_grade') IN ('C', 'D')",
		"is_cloudnative = 1",
		"LIMIT 50",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestBuildNodeHealthSQL_UnhealthAlias(t *testing.T) {
	sql, err := BuildNodeHealthSQL(NodeHealthRequest{
		Labels:     []string{"unhealth"},
		PDateStart: "20260501",
		PDateEnd:   "20260501",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "node_cpu_sys_unhealth") || !strings.Contains(sql, "node_availability_unhealth") {
		t.Fatalf("expected expanded unhealth list:\n%s", sql)
	}
}

func TestBuildNodeHealthSQL_TimeWindow(t *testing.T) {
	sql, err := BuildNodeHealthSQL(NodeHealthRequest{
		Since: "2026-05-13 00:00:00",
		Until: "2026-05-13 06:00:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "p_date >= '20260513'") {
		t.Fatalf("expected p_date derived from since: %s", sql)
	}
	if !strings.Contains(sql, "timestamp >=") || !strings.Contains(sql, "timestamp <=") {
		t.Fatalf("expected timestamp predicates: %s", sql)
	}
}

func TestBuildNodeHealthSQL_BadInput(t *testing.T) {
	cases := []NodeHealthRequest{
		{PDateStart: "2026", PDateEnd: "20260513"},
		{HourminStart: "12", HourminEnd: "1300"},
		{Since: "yesterday"},
	}
	for i, req := range cases {
		if _, err := BuildNodeHealthSQL(req); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestQueryNodeHealth_Decode(t *testing.T) {
	row := map[string]any{
		"p_date":         "20260513",
		"p_hourmin":      "0206",
		"object":         "node",
		"object_id":      "node-152.example.net",
		"label_name":     "node_availability_unhealth",
		"label_value":    `{"timestamp":1778609160,"format_time":"2026-05-13 02:06","health_grade":"C","exception":"node_availability_unhealth","service_count":3,"service_list":"[\"kwaishop-marketing-tools-service\",\"kwaishop-resource-service\",\"kwaishop-shop-center\"]"}`,
		"is_cloudnative": float64(0),
		"timestamp":      float64(1778609160),
	}
	payload := map[string]any{"data": []any{row}}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c, err := New(Config{GatewayURL: srv.URL, Token: "tok", DefaultLimit: 0})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.QueryNodeHealth(context.Background(), NodeHealthRequest{PDateStart: "20260513", PDateEnd: "20260513"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.RowCount != 1 {
		t.Fatalf("want 1 row got %d", res.RowCount)
	}
	r0 := res.Rows[0]
	if r0.HealthGrade != "C" {
		t.Fatalf("health_grade: %#v", r0)
	}
	if r0.ServiceCount != 3 {
		t.Fatalf("service_count: %d", r0.ServiceCount)
	}
	if len(r0.ServiceList) != 3 || r0.ServiceList[0] != "kwaishop-marketing-tools-service" {
		t.Fatalf("service_list parse failed: %#v", r0.ServiceList)
	}
	if r0.FormatTime != "2026-05-13 02:06" {
		t.Fatalf("format_time: %q", r0.FormatTime)
	}
	if r0.Timestamp != 1778609160 {
		t.Fatalf("timestamp: %d", r0.Timestamp)
	}
}
