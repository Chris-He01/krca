package store

import (
	"context"
	"time"
)

// SessionStore abstracts session persistence. Both SQLite and Redis implement
// this interface so callers never need to branch on the backend type.
type SessionStore interface {
	// Session CRUD
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	ListSessions(ctx context.Context, userID string, limit, offset int, sceneID string, since *time.Time) ([]*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, id string) error
	GetFullSession(ctx context.Context, id string) (*FullSessionData, error)

	// Snapshot
	SaveSnapshot(ctx context.Context, sessionID, snapshot string) error

	// Share token
	SetShareToken(ctx context.Context, sessionID, token string) error
	GetSessionByShareToken(ctx context.Context, token string) (*Session, error)

	// Messages
	AddMessage(ctx context.Context, msg *SessionMessage) error
	GetMessages(ctx context.Context, sessionID string) ([]SessionMessage, error)

	// Events (Redis may no-op)
	AddSessionEvents(ctx context.Context, events []SessionEvent) error
	GetSessionEvents(ctx context.Context, sessionID string) ([]SessionEvent, error)

	Close() error
}
