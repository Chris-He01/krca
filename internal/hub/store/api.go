package store

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"knsight-go/internal/hub/user"
)

// SessionAPI provides HTTP handlers for session CRUD.
type SessionAPI struct {
	store SessionStore
}

// NewSessionAPI creates a new SessionAPI backed by the given SessionStore.
func NewSessionAPI(s SessionStore) *SessionAPI {
	return &SessionAPI{store: s}
}

// ListSessions handles GET /v1/sessions?limit=20&offset=0&scene_id=xxx&since=1775570025&all=true.
// When all=true (used by /diagnostics 看板)，返回所有用户的 session，否则按当前登录用户过滤。
// 看板需要展示全平台的对话历史，所以走 all=true；普通会话视图按用户过滤。
func (a *SessionAPI) ListSessions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 20
		}
		u := user.FromContext(r.Context())
		sceneID := r.URL.Query().Get("scene_id")
		sinceStr := r.URL.Query().Get("since")
		all := r.URL.Query().Get("all") == "true" || r.URL.Query().Get("all") == "1"

		var sinceTime *time.Time
		if sinceStr != "" {
			if ts, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
				t := time.Unix(ts, 0)
				sinceTime = &t
			}
		}

		userFilter := u.ID
		if all {
			userFilter = "" // 看板视图：跨用户聚合
		}

		sessions, err := a.store.ListSessions(r.Context(), userFilter, limit, offset, sceneID, sinceTime)
		if err != nil {
			log.Printf("[sessions] list error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, sessions)
	}
}

// GetSession handles GET /v1/sessions/{id}.
func (a *SessionAPI) GetSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionID(r.URL.Path)
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		sess, err := a.store.GetSession(r.Context(), id)
		if err != nil {
			log.Printf("[sessions] get %s error: %v", id, err)
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, sess)
	}
}

// GetFullSession handles GET /v1/sessions/{id}/full — returns session + messages + events.
func (a *SessionAPI) GetFullSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionIDFromSub(r.URL.Path, "/full")
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		data, err := a.store.GetFullSession(r.Context(), id)
		if err != nil {
			log.Printf("[sessions] get full %s error: %v", id, err)
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, data)
	}
}

// SaveSnapshot handles POST /v1/sessions/{id}/snapshot — saves UI state snapshot.
func (a *SessionAPI) SaveSnapshot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionIDFromSub(r.URL.Path, "/snapshot")
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}

		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.store.SaveSnapshot(r.Context(), id, string(body)); err != nil {
			log.Printf("[sessions] save snapshot %s error: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

// ShareSession handles POST /v1/sessions/{id}/share — generates a share token.
func (a *SessionAPI) ShareSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionIDFromSub(r.URL.Path, "/share")
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}

		sess, err := a.store.GetSession(r.Context(), id)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		var tokenStr string
		if sess.ShareToken != nil && *sess.ShareToken != "" {
			tokenStr = *sess.ShareToken
		} else {
			tokenStr = uuid.NewString()
			if err := a.store.SetShareToken(r.Context(), id, tokenStr); err != nil {
				log.Printf("[sessions] share %s error: %v", id, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Update session object with the new token
			sess.ShareToken = &tokenStr
			if err := a.store.UpdateSession(r.Context(), sess); err != nil {
				log.Printf("[sessions] share update session %s error: %v", id, err)
			}
		}

		log.Printf("[sessions] share token created session=%s token=%s", id, tokenStr)
		writeJSON(w, map[string]string{
			"share_token": tokenStr,
			"share_url":   "/shared/" + tokenStr,
		})
	}
}

// CreateSession handles POST /v1/sessions.
func (a *SessionAPI) CreateSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var session Session
		if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if session.ID == "" {
			session.ID = uuid.NewString()
		}
		if err := a.store.CreateSession(r.Context(), &session); err != nil {
			log.Printf("[sessions] create error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, session)
	}
}

// UpdateSession handles PUT /v1/sessions/{id}.
func (a *SessionAPI) UpdateSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionID(r.URL.Path)
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}

		var session Session
		if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		session.ID = id
		if err := a.store.UpdateSession(r.Context(), &session); err != nil {
			log.Printf("[sessions] update %s error: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, session)
	}
}

// DeleteSession handles DELETE /v1/sessions/{id}.
func (a *SessionAPI) DeleteSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionID(r.URL.Path)
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		if err := a.store.DeleteSession(r.Context(), id); err != nil {
			log.Printf("[sessions] delete %s error: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

// GetMessages handles GET /v1/sessions/{id}/messages.
func (a *SessionAPI) GetMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionIDFromMessages(r.URL.Path)
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		msgs, err := a.store.GetMessages(r.Context(), id)
		if err != nil {
			log.Printf("[sessions] get messages %s error: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, msgs)
	}
}

// AddMessage handles POST /v1/sessions/{id}/messages.
func (a *SessionAPI) AddMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := extractSessionIDFromMessages(r.URL.Path)
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}

		var msg SessionMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		msg.SessionID = id
		if err := a.store.AddMessage(r.Context(), &msg); err != nil {
			log.Printf("[sessions] add message %s error: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, msg)
	}
}

// HandleSessions is a combined handler for /v1/sessions (without trailing slash).
func (a *SessionAPI) HandleSessions() http.HandlerFunc {
	list := a.ListSessions()
	create := a.CreateSession()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list(w, r)
		case http.MethodPost:
			create(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// HandleSessionByID is a combined handler for /v1/sessions/{id} and sub-paths.
func (a *SessionAPI) HandleSessionByID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/messages") {
			switch r.Method {
			case http.MethodGet:
				a.GetMessages()(w, r)
			case http.MethodPost:
				a.AddMessage()(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}
		if strings.HasSuffix(path, "/full") {
			a.GetFullSession()(w, r)
			return
		}
		if strings.HasSuffix(path, "/snapshot") {
			a.SaveSnapshot()(w, r)
			return
		}
		if strings.HasSuffix(path, "/share") {
			a.ShareSession()(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			a.GetSession()(w, r)
		case http.MethodPut:
			a.UpdateSession()(w, r)
		case http.MethodDelete:
			a.DeleteSession()(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// HandleSharedSession handles GET /v1/sessions/shared/{token} — no auth needed.
func (a *SessionAPI) HandleSharedSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		token := strings.TrimPrefix(r.URL.Path, "/v1/sessions/shared/")
		if token == "" {
			http.Error(w, "missing share token", http.StatusBadRequest)
			return
		}
		sess, err := a.store.GetSessionByShareToken(r.Context(), token)
		if err != nil {
			log.Printf("[sessions] share token not found token=%s err=%v", token, err)
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		log.Printf("[sessions] share token found token=%s session=%s", token, sess.ID)
		data, err := a.store.GetFullSession(r.Context(), sess.ID)
		if err != nil {
			log.Printf("[sessions] get full session=%s err=%v", sess.ID, err)
			http.Error(w, "failed to load session", http.StatusInternalServerError)
			return
		}
		writeJSON(w, data)
	}
}

// ─── Path helpers ───────────────────────────────────────────────────────────

func extractSessionID(path string) string {
	path = strings.TrimPrefix(path, "/v1/sessions/")
	if idx := strings.Index(path, "/"); idx >= 0 {
		path = path[:idx]
	}
	return path
}

func extractSessionIDFromMessages(path string) string {
	path = strings.TrimPrefix(path, "/v1/sessions/")
	path = strings.TrimSuffix(path, "/messages")
	if idx := strings.Index(path, "/"); idx >= 0 {
		path = path[:idx]
	}
	return path
}

func extractSessionIDFromSub(path, suffix string) string {
	path = strings.TrimPrefix(path, "/v1/sessions/")
	path = strings.TrimSuffix(path, suffix)
	if idx := strings.Index(path, "/"); idx >= 0 {
		path = path[:idx]
	}
	return path
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
