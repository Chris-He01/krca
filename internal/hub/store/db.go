package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// DB wraps SQLite for session storage.
type DB struct {
	db *sqlx.DB
}

// Session represents a chat session.
type Session struct {
	ID            string    `db:"id" json:"id"`
	Title         string    `db:"title" json:"title"`
	AgentType     string    `db:"agent_type" json:"agent_type"`
	Metadata      string    `db:"metadata" json:"metadata"` // JSON blob
	ShareToken    *string   `db:"share_token" json:"share_token,omitempty"`
	UserID        string    `db:"user_id" json:"user_id,omitempty"`
	StateSnapshot string    `db:"state_snapshot" json:"state_snapshot,omitempty"` // full UI state JSON
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// SessionMessage represents a message in a session.
type SessionMessage struct {
	ID        int64     `db:"id" json:"id"`
	SessionID string    `db:"session_id" json:"session_id"`
	Role      string    `db:"role" json:"role"` // user, assistant
	Content   string    `db:"content" json:"content"`
	Metadata  string    `db:"metadata" json:"metadata"` // JSON blob for tool calls, events, etc.
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// SessionEvent stores a raw agent event for complete replay.
type SessionEvent struct {
	ID         int64     `db:"id" json:"id"`
	SessionID  string    `db:"session_id" json:"session_id"`
	EventIndex int       `db:"event_index" json:"event_index"`
	AgentName  string    `db:"agent_name" json:"agent_name"`
	RunPath    string    `db:"run_path" json:"run_path"`    // JSON array of strings
	EventData  string    `db:"event_data" json:"event_data"` // raw PublicEvent JSON
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

// ConfigSetting stores a config key-value pair in the database.
type ConfigSetting struct {
	Key       string    `db:"key" json:"key"`
	Value     string    `db:"value" json:"value"` // JSON-encoded value
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// FullSessionData is returned by GetFullSession and shared session endpoint.
type FullSessionData struct {
	Session  *Session         `json:"session"`
	Messages []SessionMessage `json:"messages"`
	Events   []SessionEvent   `json:"events"`
}

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    agent_type TEXT NOT NULL DEFAULT 'insight',
    metadata TEXT NOT NULL DEFAULT '{}',
    share_token TEXT,
    user_id TEXT NOT NULL DEFAULT '',
    state_snapshot TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS session_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON session_messages(session_id);
CREATE TABLE IF NOT EXISTS session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    event_index INTEGER NOT NULL,
    agent_name TEXT NOT NULL DEFAULT '',
    run_path TEXT NOT NULL DEFAULT '[]',
    event_data TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_session_events_session ON session_events(session_id);
CREATE TABLE IF NOT EXISTS file_index (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    root TEXT NOT NULL,
    path TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    content TEXT,
    size INTEGER DEFAULT 0,
    modified_at DATETIME,
    synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(root, path)
);
CREATE INDEX IF NOT EXISTS idx_file_index_root ON file_index(root);
CREATE TABLE IF NOT EXISTS config_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// migrations handles adding columns to existing databases.
var migrations = []string{
	`ALTER TABLE sessions ADD COLUMN share_token TEXT`,
	`ALTER TABLE sessions ADD COLUMN user_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE sessions ADD COLUMN state_snapshot TEXT NOT NULL DEFAULT '{}'`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_share_token ON sessions(share_token) WHERE share_token IS NOT NULL`,
}

// NewDB opens (or creates) the SQLite database at dbPath and runs migrations.
func NewDB(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Run alter-table migrations (ignore errors for already-existing columns)
	for _, m := range migrations {
		db.Exec(m) // intentionally ignore errors
	}

	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// ─── Session CRUD ──────────────────────────────────────────────────────────────

// CreateSession inserts a new session.
func (d *DB) CreateSession(session *Session) error {
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}
	if session.Metadata == "" {
		session.Metadata = "{}"
	}
	if session.AgentType == "" {
		session.AgentType = "insight"
	}
	if session.StateSnapshot == "" {
		session.StateSnapshot = "{}"
	}

	_, err := d.db.Exec(
		`INSERT INTO sessions (id, title, agent_type, metadata, user_id, state_snapshot, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.Title, session.AgentType, session.Metadata,
		session.UserID, session.StateSnapshot,
		session.CreatedAt, session.UpdatedAt,
	)
	return err
}

// GetSession retrieves a session by ID.
func (d *DB) GetSession(id string) (*Session, error) {
	var s Session
	if err := d.db.Get(&s, `SELECT * FROM sessions WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSessions returns sessions ordered by updated_at desc.
func (d *DB) ListSessions(limit, offset int) ([]Session, error) {
	if limit <= 0 {
		limit = 20
	}
	var sessions []Session
	if err := d.db.Select(&sessions,
		`SELECT * FROM sessions ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	); err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListSessionsByUser returns sessions for a specific user ordered by updated_at desc.
// If userID is empty, returns all sessions (backward compatible).
func (d *DB) ListSessionsByUser(userID string, limit, offset int) ([]Session, error) {
	if userID == "" {
		return d.ListSessions(limit, offset)
	}
	if limit <= 0 {
		limit = 20
	}
	var sessions []Session
	if err := d.db.Select(&sessions,
		`SELECT * FROM sessions WHERE user_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	); err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListSessionsByUserAndScene returns sessions filtered by user + optional scene_id + optional since timestamp.
// scene_id is matched against the metadata JSON field. since is a minimum created_at filter.
func (d *DB) ListSessionsByUserAndScene(userID, sceneID string, since *time.Time, limit, offset int) ([]Session, error) {
	if sceneID == "" && since == nil {
		return d.ListSessionsByUser(userID, limit, offset)
	}
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT * FROM sessions WHERE 1=1`
	args := []interface{}{}

	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	if sceneID != "" {
		// Match scene_id in the metadata JSON blob using SQLite JSON functions
		// Fallback: match agent_type prefix for compatibility
		query += ` AND (json_extract(metadata, '$.scene_id') = ? OR agent_type = ?)`
		args = append(args, sceneID, "scene/"+sceneID)
	}
	if since != nil {
		query += ` AND created_at >= ?`
		args = append(args, since.UTC())
	}

	query += ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	var sessions []Session
	if err := d.db.Select(&sessions, query, args...); err != nil {
		return nil, err
	}
	return sessions, nil
}

// UpdateSession updates an existing session's title, agent_type, metadata, and updated_at.
func (d *DB) UpdateSession(session *Session) error {
	session.UpdatedAt = time.Now().UTC()
	if session.Metadata == "" {
		session.Metadata = "{}"
	}
	_, err := d.db.Exec(
		`UPDATE sessions SET title = ?, agent_type = ?, metadata = ?, updated_at = ? WHERE id = ?`,
		session.Title, session.AgentType, session.Metadata, session.UpdatedAt, session.ID,
	)
	return err
}

// DeleteSession removes a session and its messages (via ON DELETE CASCADE).
func (d *DB) DeleteSession(id string) error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// UpdateSessionSnapshot saves the full UI state snapshot for a session.
func (d *DB) UpdateSessionSnapshot(id, snapshot string) error {
	_, err := d.db.Exec(
		`UPDATE sessions SET state_snapshot = ?, updated_at = ? WHERE id = ?`,
		snapshot, time.Now().UTC(), id,
	)
	return err
}

// SetShareToken sets the share token on a session.
func (d *DB) SetShareToken(id, token string) error {
	_, err := d.db.Exec(
		`UPDATE sessions SET share_token = ?, updated_at = ? WHERE id = ?`,
		token, time.Now().UTC(), id,
	)
	return err
}

// GetSessionByShareToken retrieves a session by its share token.
func (d *DB) GetSessionByShareToken(token string) (*Session, error) {
	var s Session
	if err := d.db.Get(&s, `SELECT * FROM sessions WHERE share_token = ?`, token); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetFullSession returns a session along with all its messages and events.
// 空消息/事件返回 [] 而不是 null，与 /v1/sessions/{id}/messages 对齐。
func (d *DB) GetFullSession(id string) (*FullSessionData, error) {
	sess, err := d.GetSession(id)
	if err != nil {
		return nil, err
	}
	msgs, err := d.GetMessages(id)
	if err != nil {
		return nil, err
	}
	if msgs == nil {
		msgs = []SessionMessage{}
	}
	events, err := d.GetSessionEvents(id)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []SessionEvent{}
	}
	return &FullSessionData{
		Session:  sess,
		Messages: msgs,
		Events:   events,
	}, nil
}

// ─── Session Messages ──────────────────────────────────────────────────────────

// AddMessage inserts a new message for a session.
func (d *DB) AddMessage(msg *SessionMessage) error {
	now := time.Now().UTC()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}
	if msg.Metadata == "" {
		msg.Metadata = "{}"
	}

	result, err := d.db.Exec(
		`INSERT INTO session_messages (session_id, role, content, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		msg.SessionID, msg.Role, msg.Content, msg.Metadata, msg.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	msg.ID = id
	return nil
}

// GetMessages returns all messages for a session ordered by created_at asc.
func (d *DB) GetMessages(sessionID string) ([]SessionMessage, error) {
	var msgs []SessionMessage
	if err := d.db.Select(&msgs,
		`SELECT * FROM session_messages WHERE session_id = ? ORDER BY created_at ASC`,
		sessionID,
	); err != nil {
		return nil, err
	}
	return msgs, nil
}

// DeleteMessages removes all messages for a session.
func (d *DB) DeleteMessages(sessionID string) error {
	_, err := d.db.Exec(`DELETE FROM session_messages WHERE session_id = ?`, sessionID)
	return err
}

// ─── Session Events ───────────────────────────────────────────────────────────

// AddSessionEvents bulk-inserts agent events for a session.
func (d *DB) AddSessionEvents(events []SessionEvent) error {
	if len(events) == 0 {
		return nil
	}
	// Use a transaction for bulk insert
	tx, err := d.db.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	stmt, err := tx.Prepare(
		`INSERT INTO session_events (session_id, event_index, agent_name, run_path, event_data, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ev := range events {
		if ev.CreatedAt.IsZero() {
			ev.CreatedAt = now
		}
		if ev.RunPath == "" {
			ev.RunPath = "[]"
		}
		if _, err = stmt.Exec(ev.SessionID, ev.EventIndex, ev.AgentName, ev.RunPath, ev.EventData, ev.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSessionEvents returns all agent events for a session ordered by event_index asc.
func (d *DB) GetSessionEvents(sessionID string) ([]SessionEvent, error) {
	var events []SessionEvent
	if err := d.db.Select(&events,
		`SELECT * FROM session_events WHERE session_id = ? ORDER BY event_index ASC`,
		sessionID,
	); err != nil {
		return nil, err
	}
	return events, nil
}

// DeleteSessionEvents removes all events for a session.
func (d *DB) DeleteSessionEvents(sessionID string) error {
	_, err := d.db.Exec(`DELETE FROM session_events WHERE session_id = ?`, sessionID)
	return err
}

// ─── Config Settings ──────────────────────────────────────────────────────────

// UpsertConfigSetting inserts or updates a config setting.
func (d *DB) UpsertConfigSetting(key, value string) error {
	_, err := d.db.Exec(
		`INSERT INTO config_settings (key, value, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC(),
	)
	return err
}

// GetConfigSetting retrieves a config setting by key.
func (d *DB) GetConfigSetting(key string) (string, error) {
	var s ConfigSetting
	if err := d.db.Get(&s, `SELECT * FROM config_settings WHERE key = ?`, key); err != nil {
		return "", err
	}
	return s.Value, nil
}

// GetAllConfigSettings returns all config settings as a map.
func (d *DB) GetAllConfigSettings() (map[string]string, error) {
	var settings []ConfigSetting
	if err := d.db.Select(&settings, `SELECT * FROM config_settings`); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}
