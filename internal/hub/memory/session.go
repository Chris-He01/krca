package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Message represents a single chat message in a session.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Session holds in-memory conversation history for a single channel:chat_id.
type Session struct {
	Key      string    `json:"key"`
	Messages []Message `json:"messages"`
}

// SessionManager manages JSONL-persisted sessions with in-memory cache.
type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	baseDir     string
	maxMessages int
}

func NewSessionManager(baseDir string, maxMessages int) *SessionManager {
	if maxMessages <= 0 {
		maxMessages = 50
	}
	return &SessionManager{
		sessions:    make(map[string]*Session),
		baseDir:     baseDir,
		maxMessages: maxMessages,
	}
}

func (sm *SessionManager) sessionFile(key string) string {
	safe := strings.ReplaceAll(key, ":", "_")
	safe = strings.ReplaceAll(safe, "/", "_")
	return filepath.Join(sm.baseDir, "sessions", safe+".jsonl")
}

// GetOrCreate returns an existing session or creates a new one, loading from disk if needed.
func (sm *SessionManager) GetOrCreate(key string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, ok := sm.sessions[key]; ok {
		return s
	}

	s := &Session{Key: key}
	// Try loading from disk
	filePath := sm.sessionFile(key)
	if f, err := os.Open(filePath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var msg Message
			if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil {
				s.Messages = append(s.Messages, msg)
			}
		}
	}

	sm.sessions[key] = s
	return s
}

// AddMessage appends a message to the session.
func (sm *SessionManager) AddMessage(key, role, content string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[key]
	if !ok {
		s = &Session{Key: key}
		sm.sessions[key] = s
	}
	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC(),
	})
}

// GetHistory returns the last maxMessages for LLM context.
func (sm *SessionManager) GetHistory(key string, max int) []Message {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	s, ok := sm.sessions[key]
	if !ok {
		return nil
	}
	if max <= 0 {
		max = sm.maxMessages
	}
	msgs := s.Messages
	if len(msgs) > max {
		msgs = msgs[len(msgs)-max:]
	}
	result := make([]Message, len(msgs))
	copy(result, msgs)
	return result
}

// Save persists the session to disk as JSONL (full rewrite).
func (sm *SessionManager) Save(key string) error {
	sm.mu.RLock()
	s, ok := sm.sessions[key]
	if !ok {
		sm.mu.RUnlock()
		return nil
	}
	msgs := make([]Message, len(s.Messages))
	copy(msgs, s.Messages)
	sm.mu.RUnlock()

	filePath := sm.sessionFile(key)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, msg := range msgs {
		if err := enc.Encode(msg); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
	}
	return nil
}

// Delete removes a session from memory and disk.
func (sm *SessionManager) Delete(key string) error {
	sm.mu.Lock()
	delete(sm.sessions, key)
	sm.mu.Unlock()

	filePath := sm.sessionFile(key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
