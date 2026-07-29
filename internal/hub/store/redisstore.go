package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
)

// RedisStore wraps a kedis client with a key prefix scheme.
// Keys follow the pattern: {prefix}:{category}:{id}
// e.g. prod:file:skills/hello.md  or  prod:session:abc-123
//
// RedisStore implements SessionStore so it can be used interchangeably with SQLiteStore.
type RedisStore struct {
	client *redis.Client
	prefix string // e.g. "prod"
}

const llmRoundTTL = 365 * 24 * time.Hour

// NewRedisStore creates a RedisStore using a Redis URL and key prefix.
func NewRedisStore(resourceName, prefix string) (*RedisStore, error) {
	opts, err := redis.ParseURL(resourceName)
	if err != nil {
		return nil, fmt.Errorf("redis URL: %w", err)
	}
	opts.DialTimeout = 3 * time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.IdleTimeout = time.Minute
	opts.IdleCheckFrequency = time.Minute
	opts.MinIdleConns = 2
	opts.PoolSize = 10
	client := redis.NewClient(opts)
	return &RedisStore{client: client, prefix: prefix}, nil
}

// Client returns the underlying Redis client for direct operations.
func (r *RedisStore) Client() *redis.Client {
	return r.client
}

// Prefix returns the key prefix (e.g., "prod").
func (r *RedisStore) Prefix() string {
	return r.prefix
}

// Ping writes and reads a test key to verify Redis connectivity end-to-end.
func (r *RedisStore) Ping(ctx context.Context) error {
	testKey := r.key("ping", "startup")
	if err := r.client.Set(testKey, "1", time.Hour).Err(); err != nil {
		return fmt.Errorf("SET failed: %w", err)
	}
	val, err := r.client.Get(testKey).Result()
	if err != nil {
		return fmt.Errorf("GET after SET failed: %w", err)
	}
	if val != "1" {
		return fmt.Errorf("GET returned unexpected value: %q", val)
	}
	return nil
}

// AppendLLMRound appends one serialized LLM request/response record.
// Records are retained for one year under
// {prefix}:llm_rounds:{session_id}:{run_id}. Callers without a session keep
// using the legacy {prefix}:llm_rounds:{run_id} form.
//
// To avoid relying on Redis key-space SCAN (which is unavailable on some
// cluster proxy implementations), each write also adds run_id to the index
// set {prefix}:idx:llm_rounds:{session_id}.  Readers enumerate run_ids via
// SSCAN on that set instead of SCAN … MATCH.
func (r *RedisStore) AppendLLMRound(_ context.Context, sessionID, runID, payload string) error {
	if runID == "" {
		return fmt.Errorf("redis append llm round: empty run_id")
	}
	k := r.key("llm_rounds", llmRoundRedisID(sessionID, runID))
	if err := r.client.RPush(k, payload).Err(); err != nil {
		return fmt.Errorf("redis rpush llm round %s: %w", runID, err)
	}
	if err := r.client.Expire(k, llmRoundTTL).Err(); err != nil {
		return fmt.Errorf("redis expire llm round %s: %w", runID, err)
	}
	// Maintain a lightweight index set so readers can enumerate run_ids
	// without SCAN.  Use the session_id as the index key; fall back to a
	// global "nosession" bucket when there is no session.
	idxID := sessionID
	if idxID == "" {
		idxID = "nosession"
	}
	idxKey := r.indexKey("llm_rounds:" + idxID)
	if err := r.client.SAdd(idxKey, runID).Err(); err != nil {
		// Non-fatal: index miss only affects debug tooling, not runtime correctness.
		log.Printf("[redis] sadd llm_rounds index FAILED session=%s run=%s: %v", sessionID, runID, err)
	} else {
		// Keep the index set alive as long as the newest round entry.
		if err := r.client.Expire(idxKey, llmRoundTTL).Err(); err != nil {
			log.Printf("[redis] expire llm_rounds index FAILED session=%s: %v", sessionID, err)
		}
	}
	return nil
}

func llmRoundRedisID(sessionID, runID string) string {
	if sessionID == "" {
		return runID
	}
	return sessionID + ":" + runID
}

// scanSet iterates over all members of a Redis set using SScan (SMembers not available).
func (r *RedisStore) scanSet(key string) ([]string, error) {
	var all []string
	var cursor uint64
	for {
		members, next, err := r.client.SScan(key, cursor, "*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("redis sscan %s: %w", key, err)
		}
		all = append(all, members...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return all, nil
}

// key returns the full Redis key for a given category and id.
func (r *RedisStore) key(category, id string) string {
	return r.prefix + ":" + category + ":" + id
}

// indexKey returns the Redis set key that tracks all ids in a category.
func (r *RedisStore) indexKey(category string) string {
	return r.prefix + ":idx:" + category
}

// ─── File operations (not part of SessionStore) ──────────────────────────────

// SetFile stores a file's content in Redis and registers it in the category index.
// category is e.g. "skills" or "memory", relPath is the path relative to the category root.
func (r *RedisStore) SetFile(ctx context.Context, category, relPath, content string) error {
	k := r.key("file:"+category, relPath)
	if err := r.client.Set(k, content, 0).Err(); err != nil {
		log.Printf("[redis] set file FAILED key=%s: %v", k, err)
		return fmt.Errorf("redis set file %s: %w", k, err)
	}
	log.Printf("[redis] set file OK key=%s (%d bytes)", k, len(content))
	// Track in index set so we can enumerate files without Keys()
	if err := r.client.SAdd(r.indexKey("file:"+category), relPath).Err(); err != nil {
		log.Printf("[redis] SAdd index %s %s: %v", category, relPath, err)
	}
	return nil
}

// GetFile retrieves a file's content from Redis.
func (r *RedisStore) GetFile(ctx context.Context, category, relPath string) (string, error) {
	k := r.key("file:"+category, relPath)
	return r.client.Get(k).Result()
}

// ListFiles returns all relative paths tracked under a category.
func (r *RedisStore) ListFiles(ctx context.Context, category string) ([]string, error) {
	return r.scanSet(r.indexKey("file:" + category))
}

// SyncFilesToDir downloads all Redis files in a category to a local directory.
// Existing local files are not overwritten unless the Redis content differs.
func (r *RedisStore) SyncFilesToDir(ctx context.Context, category, localDir string) error {
	paths, err := r.ListFiles(ctx, category)
	if err != nil {
		return err
	}
	written := 0
	for _, relPath := range paths {
		content, err := r.GetFile(ctx, category, relPath)
		if err != nil {
			log.Printf("[redis] get file %s/%s: %v", category, relPath, err)
			continue
		}
		dest := filepath.Join(localDir, filepath.FromSlash(relPath))
		// Check if local file already has the same content
		existing, readErr := os.ReadFile(dest)
		if readErr == nil && string(existing) == content {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			log.Printf("[redis] mkdir %s: %v", filepath.Dir(dest), err)
			continue
		}
		if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
			log.Printf("[redis] write file %s: %v", dest, err)
			continue
		}
		log.Printf("[redis] sync restored key=%s → %s", r.key("file:"+category, relPath), dest)
		written++
	}
	log.Printf("[redis] sync %s → %s: %d/%d files written", category, localDir, written, len(paths))
	return nil
}

// DeleteFile removes a single file entry from Redis (key + index membership).
// If relPath identifies a directory prefix, callers should use DeleteFilePrefix instead.
func (r *RedisStore) DeleteFile(ctx context.Context, category, relPath string) error {
	k := r.key("file:"+category, relPath)
	if err := r.client.Del(k).Err(); err != nil {
		log.Printf("[redis] del file FAILED key=%s: %v", k, err)
		return fmt.Errorf("redis del file %s: %w", k, err)
	}
	if err := r.client.SRem(r.indexKey("file:"+category), relPath).Err(); err != nil {
		log.Printf("[redis] SRem index %s %s: %v", category, relPath, err)
	}
	log.Printf("[redis] del file OK key=%s", k)
	return nil
}

// DeleteFilePrefix removes every file under relPath (treating it as a directory).
// All matching paths in the category index are scanned and deleted.
func (r *RedisStore) DeleteFilePrefix(ctx context.Context, category, relPath string) error {
	paths, err := r.ListFiles(ctx, category)
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(relPath, "/") + "/"
	for _, p := range paths {
		if p == relPath || strings.HasPrefix(p, prefix) {
			if err := r.DeleteFile(ctx, category, p); err != nil {
				log.Printf("[redis] delete prefix entry %s/%s: %v", category, p, err)
			}
		}
	}
	return nil
}

// SyncDirToRedis uploads all files under localDir to Redis under the given category.
func (r *RedisStore) SyncDirToRedis(ctx context.Context, category, localDir string) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		// Skip hidden files
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable
		}
		relPath, _ := filepath.Rel(localDir, path)
		relPath = filepath.ToSlash(relPath)
		if setErr := r.SetFile(ctx, category, relPath, string(content)); setErr != nil {
			log.Printf("[redis] set file %s/%s: %v", category, relPath, setErr)
		}
		return nil
	})
}

// ─── SessionStore implementation ────────────────────────────────────────────

// setSessionRaw stores a JSON blob for a session and adds it to the session index.
func (r *RedisStore) setSessionRaw(sessionID, jsonBlob string) error {
	k := r.key("session", sessionID)
	if err := r.client.Set(k, jsonBlob, 0).Err(); err != nil {
		log.Printf("[redis] set session FAILED %s: %v", sessionID, err)
		return fmt.Errorf("redis set session %s: %w", sessionID, err)
	}
	log.Printf("[redis] set session OK %s", sessionID)
	if err := r.client.SAdd(r.indexKey("session"), sessionID).Err(); err != nil {
		log.Printf("[redis] sadd session index FAILED %s: %v", sessionID, err)
	}
	return nil
}

func (r *RedisStore) CreateSession(_ context.Context, session *Session) error {
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}
	b, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return r.setSessionRaw(session.ID, string(b))
}

func (r *RedisStore) GetSession(_ context.Context, sessionID string) (*Session, error) {
	blob, err := r.client.Get(r.key("session", sessionID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis get session %s: %w", sessionID, err)
	}
	var sess Session
	if err := json.Unmarshal([]byte(blob), &sess); err != nil {
		return nil, fmt.Errorf("redis parse session %s: %w", sessionID, err)
	}
	return &sess, nil
}

func (r *RedisStore) ListSessions(_ context.Context, userID string, limit, offset int, sceneID string, since *time.Time) ([]*Session, error) {
	ids, err := r.scanSet(r.indexKey("session"))
	if err != nil {
		return nil, err
	}
	sessions := make([]*Session, 0, len(ids))
	for _, id := range ids {
		sess, err := r.GetSession(context.Background(), id)
		if err != nil {
			continue
		}
		// Filter by user if specified
		if userID != "" && sess.UserID != userID {
			continue
		}
		// Filter by scene_id (match agent_type prefix or metadata JSON)
		if sceneID != "" {
			if sess.AgentType != "scene/"+sceneID && !strings.Contains(sess.Metadata, `"scene_id":"`+sceneID+`"`) {
				continue
			}
		}
		// Filter by since timestamp
		if since != nil && sess.CreatedAt.Before(*since) {
			continue
		}
		sessions = append(sessions, sess)
	}
	// Sort by UpdatedAt descending
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && sessions[j].UpdatedAt.After(sessions[j-1].UpdatedAt); j-- {
			sessions[j], sessions[j-1] = sessions[j-1], sessions[j]
		}
	}
	// Apply offset/limit
	if offset > len(sessions) {
		offset = len(sessions)
	}
	end := offset + limit
	if end > len(sessions) {
		end = len(sessions)
	}
	return sessions[offset:end], nil
}

func (r *RedisStore) UpdateSession(ctx context.Context, session *Session) error {
	// Read existing session first, then merge non-zero fields to avoid overwriting
	existing, err := r.GetSession(ctx, session.ID)
	if err != nil {
		// Session doesn't exist, just create it
		return r.CreateSession(ctx, session)
	}

	// Merge: only update fields that are non-zero in the input
	if session.Title != "" {
		existing.Title = session.Title
	}
	if session.AgentType != "" {
		existing.AgentType = session.AgentType
	}
	if session.Metadata != "" && session.Metadata != "{}" {
		existing.Metadata = session.Metadata
	}
	if session.UserID != "" {
		existing.UserID = session.UserID
	}
	if session.ShareToken != nil {
		existing.ShareToken = session.ShareToken
	}
	if session.StateSnapshot != "" && session.StateSnapshot != "{}" {
		existing.StateSnapshot = session.StateSnapshot
	}
	existing.UpdatedAt = time.Now().UTC()

	b, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return r.setSessionRaw(existing.ID, string(b))
}

func (r *RedisStore) DeleteSession(_ context.Context, sessionID string) error {
	r.client.Del(r.key("session", sessionID))
	r.client.Del(r.key("snapshot", sessionID))
	r.client.Del(r.key("messages", sessionID))
	r.client.Del(r.key("events", sessionID))
	r.client.SRem(r.indexKey("session"), sessionID)
	return nil
}

func (r *RedisStore) GetFullSession(ctx context.Context, sessionID string) (*FullSessionData, error) {
	sess, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	// Snapshot may be stored separately
	if snap, err := r.client.Get(r.key("snapshot", sessionID)).Result(); err == nil && snap != "" {
		sess.StateSnapshot = snap
	}
	// Surface any read errors instead of silently swallowing them — in the past
	// 排查 /v1/sessions/{id}/full 显示空消息时一无所知就是因为这两次 _ 把错误吃了。
	msgs, mErr := r.GetMessages(ctx, sessionID)
	if mErr != nil {
		log.Printf("[redis] GetFullSession get messages %s error: %v", sessionID, mErr)
	}
	if msgs == nil {
		msgs = []SessionMessage{}
	}
	events, eErr := r.GetSessionEvents(ctx, sessionID)
	if eErr != nil {
		log.Printf("[redis] GetFullSession get events %s error: %v", sessionID, eErr)
	}
	if events == nil {
		events = []SessionEvent{}
	}
	return &FullSessionData{Session: sess, Messages: msgs, Events: events}, nil
}

func (r *RedisStore) SaveSnapshot(_ context.Context, sessionID, snapshot string) error {
	k := r.key("snapshot", sessionID)
	if err := r.client.Set(k, snapshot, 0).Err(); err != nil {
		log.Printf("[redis] set snapshot FAILED %s: %v", sessionID, err)
		return fmt.Errorf("redis set snapshot %s: %w", sessionID, err)
	}
	return nil
}

func (r *RedisStore) SetShareToken(_ context.Context, sessionID, token string) error {
	k := r.key("sharetoken", token)
	if err := r.client.Set(k, sessionID, 0).Err(); err != nil {
		log.Printf("[redis] set sharetoken FAILED token=%s: %v", token, err)
		return fmt.Errorf("redis set sharetoken %s: %w", token, err)
	}
	log.Printf("[redis] set sharetoken OK token=%s session=%s", token, sessionID)
	return nil
}

func (r *RedisStore) GetSessionByShareToken(ctx context.Context, token string) (*Session, error) {
	k := r.key("sharetoken", token)
	sessionID, err := r.client.Get(k).Result()
	if err != nil {
		return nil, fmt.Errorf("share token not found: %s", token)
	}
	return r.GetSession(ctx, sessionID)
}

func (r *RedisStore) AddMessage(_ context.Context, msg *SessionMessage) error {
	k := r.key("messages", msg.SessionID)
	var msgs []SessionMessage
	if blob, err := r.client.Get(k).Result(); err == nil {
		_ = json.Unmarshal([]byte(blob), &msgs)
	}
	msg.ID = int64(len(msgs) + 1)
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.Metadata == "" {
		msg.Metadata = "{}"
	}
	msgs = append(msgs, *msg)
	b, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("redis marshal messages %s: %w", msg.SessionID, err)
	}
	if err := r.client.Set(k, string(b), 0).Err(); err != nil {
		return fmt.Errorf("redis set messages %s: %w", msg.SessionID, err)
	}
	return nil
}

func (r *RedisStore) GetMessages(_ context.Context, sessionID string) ([]SessionMessage, error) {
	k := r.key("messages", sessionID)
	blob, err := r.client.Get(k).Result()
	if err != nil {
		return []SessionMessage{}, nil // no messages yet is not an error
	}
	var msgs []SessionMessage
	if err := json.Unmarshal([]byte(blob), &msgs); err != nil {
		return nil, fmt.Errorf("redis parse messages %s: %w", sessionID, err)
	}
	return msgs, nil
}

// AddSessionEvents appends events to the session's event list in Redis.
func (r *RedisStore) AddSessionEvents(_ context.Context, events []SessionEvent) error {
	if len(events) == 0 {
		return nil
	}
	sessionID := events[0].SessionID
	k := r.key("events", sessionID)
	var existing []SessionEvent
	if blob, err := r.client.Get(k).Result(); err == nil {
		_ = json.Unmarshal([]byte(blob), &existing)
	}
	now := time.Now().UTC()
	for i := range events {
		events[i].ID = int64(len(existing) + i + 1)
		if events[i].CreatedAt.IsZero() {
			events[i].CreatedAt = now
		}
	}
	existing = append(existing, events...)
	b, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("redis marshal events %s: %w", sessionID, err)
	}
	if err := r.client.Set(k, string(b), 0).Err(); err != nil {
		return fmt.Errorf("redis set events %s: %w", sessionID, err)
	}
	return nil
}

// GetSessionEvents retrieves all events for a session from Redis.
func (r *RedisStore) GetSessionEvents(_ context.Context, sessionID string) ([]SessionEvent, error) {
	k := r.key("events", sessionID)
	blob, err := r.client.Get(k).Result()
	if err != nil {
		return []SessionEvent{}, nil
	}
	var events []SessionEvent
	if err := json.Unmarshal([]byte(blob), &events); err != nil {
		return nil, fmt.Errorf("redis parse events %s: %w", sessionID, err)
	}
	return events, nil
}

func (r *RedisStore) Close() error {
	return nil
}

// ─── Migration helpers (not part of SessionStore) ───────────────────────────

// MigrateSessionsFromDB reads all sessions from SQLite and writes missing ones to Redis.
func (r *RedisStore) MigrateSessionsFromDB(ctx context.Context, db *DB) error {
	sessions, err := db.ListSessions(10000, 0)
	if err != nil {
		return fmt.Errorf("migrate: list sqlite sessions: %w", err)
	}
	migrated := 0
	for _, sess := range sessions {
		k := r.key("session", sess.ID)
		if exists, _ := r.client.Exists(k).Result(); exists > 0 {
			continue
		}
		b, err := json.Marshal(sess)
		if err != nil {
			continue
		}
		if err := r.setSessionRaw(sess.ID, string(b)); err != nil {
			log.Printf("[redis] migrate session %s: %v", sess.ID, err)
			continue
		}
		if sess.ShareToken != nil && *sess.ShareToken != "" {
			_ = r.SetShareToken(ctx, sess.ID, *sess.ShareToken)
		}
		if sess.StateSnapshot != "" {
			_ = r.SaveSnapshot(ctx, sess.ID, sess.StateSnapshot)
		}
		migrated++
	}
	log.Printf("[redis] migrated %d/%d sessions from SQLite to Redis", migrated, len(sessions))
	return nil
}

// RestoreSessionsToDB reads all sessions from Redis and inserts missing ones into SQLite.
func (r *RedisStore) RestoreSessionsToDB(ctx context.Context, db *DB) error {
	ids, err := r.scanSet(r.indexKey("session"))
	if err != nil {
		return fmt.Errorf("redis sscan sessions: %w", err)
	}
	restored := 0
	for _, id := range ids {
		if _, err := db.GetSession(id); err == nil {
			continue
		}
		blob, err := r.client.Get(r.key("session", id)).Result()
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal([]byte(blob), &sess); err != nil {
			log.Printf("[redis] restore session %s: parse error: %v", id, err)
			continue
		}
		if err := db.CreateSession(&sess); err != nil {
			log.Printf("[redis] restore session %s: db insert: %v", id, err)
			continue
		}
		if sess.ShareToken != nil && *sess.ShareToken != "" {
			_ = db.SetShareToken(id, *sess.ShareToken)
		}
		snap, snapErr := r.client.Get(r.key("snapshot", id)).Result()
		if snapErr == nil && snap != "" {
			_ = db.UpdateSessionSnapshot(id, snap)
		}
		restored++
	}
	log.Printf("[redis] restored %d/%d sessions to SQLite", restored, len(ids))
	return nil
}
