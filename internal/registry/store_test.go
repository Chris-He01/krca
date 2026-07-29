package registry

import (
	"testing"
	"time"

	"knsight-go/internal/protocol"
)

func TestStoreRegisterHeartbeatCleanup(t *testing.T) {
	store := NewStore()
	card := protocol.AgentCard{
		ID:           "agent-1",
		Name:         "InspectAgent",
		Version:      "0.1.0",
		Description:  "test agent",
		Capabilities: []string{"inspect"},
		Endpoint:     "http://localhost:9000",
	}

	rec := store.Register(card, 1)
	if rec.Card.ID != card.ID {
		t.Fatalf("expected agent id %s, got %s", card.ID, rec.Card.ID)
	}

	if _, ok := store.Heartbeat(card.ID); !ok {
		t.Fatalf("expected heartbeat to succeed")
	}

	time.Sleep(1200 * time.Millisecond)
	removed := store.CleanupExpired()
	if removed != 1 {
		t.Fatalf("expected 1 expired agent, got %d", removed)
	}
}
