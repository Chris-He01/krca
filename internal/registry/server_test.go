package registry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"knsight-go/internal/protocol"
)

func TestRegistryServerRegisterAndList(t *testing.T) {
	store := NewStore()
	srv := NewServer(store)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	card := protocol.AgentCard{
		ID:           "agent-1",
		Name:         "InspectAgent",
		Version:      "0.1.0",
		Description:  "inspect agent",
		Capabilities: []string{"inspect"},
		Endpoint:     "http://localhost:9000",
	}
	payload := map[string]any{
		"card":        card,
		"ttl_seconds": 10,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/v1/registry/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	_ = resp.Body.Close()

	listResp, err := http.Get(ts.URL + "/v1/registry/agents")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected list status: %s", listResp.Status)
	}
	var records []AgentRecord
	if err := json.NewDecoder(listResp.Body).Decode(&records); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Card.ID != card.ID {
		t.Fatalf("unexpected agent id %s", records[0].Card.ID)
	}
}
