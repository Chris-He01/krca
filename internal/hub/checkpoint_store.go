package hub

import (
	"context"
	"sync"
)

type MemoryCheckPointStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryCheckPointStore() *MemoryCheckPointStore {
	return &MemoryCheckPointStore{data: make(map[string][]byte)}
}

func (s *MemoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[checkPointID]
	if !ok {
		return nil, false, nil
	}
	copied := make([]byte, len(val))
	copy(copied, val)
	return copied, true, nil
}

func (s *MemoryCheckPointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]byte, len(checkPoint))
	copy(copied, checkPoint)
	s.data[checkPointID] = copied
	return nil
}

func (s *MemoryCheckPointStore) Has(checkPointID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[checkPointID]
	return ok
}
