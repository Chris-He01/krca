package store

import (
	"context"
	"time"
)

// SQLiteStore wraps *DB to implement SessionStore.
type SQLiteStore struct {
	db *DB
}

// NewSQLiteStore creates a SessionStore backed by SQLite.
func NewSQLiteStore(db *DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// DB returns the underlying *DB for non-session operations (config, filetree, etc.).
func (s *SQLiteStore) DB() *DB { return s.db }

func (s *SQLiteStore) CreateSession(_ context.Context, session *Session) error {
	return s.db.CreateSession(session)
}

func (s *SQLiteStore) GetSession(_ context.Context, id string) (*Session, error) {
	return s.db.GetSession(id)
}

func (s *SQLiteStore) ListSessions(_ context.Context, userID string, limit, offset int, sceneID string, since *time.Time) ([]*Session, error) {
	rows, err := s.db.ListSessionsByUserAndScene(userID, sceneID, since, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]*Session, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out, nil
}

func (s *SQLiteStore) UpdateSession(_ context.Context, session *Session) error {
	return s.db.UpdateSession(session)
}

func (s *SQLiteStore) DeleteSession(_ context.Context, id string) error {
	return s.db.DeleteSession(id)
}

func (s *SQLiteStore) GetFullSession(_ context.Context, id string) (*FullSessionData, error) {
	return s.db.GetFullSession(id)
}

func (s *SQLiteStore) SaveSnapshot(_ context.Context, sessionID, snapshot string) error {
	return s.db.UpdateSessionSnapshot(sessionID, snapshot)
}

func (s *SQLiteStore) SetShareToken(_ context.Context, sessionID, token string) error {
	return s.db.SetShareToken(sessionID, token)
}

func (s *SQLiteStore) GetSessionByShareToken(_ context.Context, token string) (*Session, error) {
	return s.db.GetSessionByShareToken(token)
}

func (s *SQLiteStore) AddMessage(_ context.Context, msg *SessionMessage) error {
	return s.db.AddMessage(msg)
}

func (s *SQLiteStore) GetMessages(_ context.Context, sessionID string) ([]SessionMessage, error) {
	return s.db.GetMessages(sessionID)
}

func (s *SQLiteStore) AddSessionEvents(_ context.Context, events []SessionEvent) error {
	return s.db.AddSessionEvents(events)
}

func (s *SQLiteStore) GetSessionEvents(_ context.Context, sessionID string) ([]SessionEvent, error) {
	return s.db.GetSessionEvents(sessionID)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
