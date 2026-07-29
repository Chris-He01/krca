package registry

import (
	"sync"
	"time"

	"knsight-go/internal/protocol"
)

type AgentRecord struct {
	Card       protocol.AgentCard `json:"card"`
	TTLSeconds int                `json:"ttl_seconds"`
	LastSeen   time.Time          `json:"last_seen"`
}

type Store struct {
	mu     sync.RWMutex
	agents map[string]AgentRecord
}

func NewStore() *Store {
	return &Store{agents: make(map[string]AgentRecord)}
}

func (s *Store) Register(card protocol.AgentCard, ttlSeconds int) AgentRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := AgentRecord{Card: card, TTLSeconds: ttlSeconds, LastSeen: time.Now().UTC()}
	s.agents[card.ID] = rec
	return rec
}

func (s *Store) Heartbeat(agentID string) (AgentRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.agents[agentID]
	if !ok {
		return AgentRecord{}, false
	}
	rec.LastSeen = time.Now().UTC()
	s.agents[agentID] = rec
	return rec, true
}

func (s *Store) List() []AgentRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentRecord, 0, len(s.agents))
	for _, rec := range s.agents {
		out = append(out, rec)
	}
	return out
}

func (s *Store) Get(agentID string) (AgentRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.agents[agentID]
	return rec, ok
}

func (s *Store) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	now := time.Now().UTC()
	for id, rec := range s.agents {
		if rec.TTLSeconds <= 0 {
			continue
		}
		expires := rec.LastSeen.Add(time.Duration(rec.TTLSeconds) * time.Second)
		if now.After(expires) {
			delete(s.agents, id)
			removed++
		}
	}
	return removed
}
