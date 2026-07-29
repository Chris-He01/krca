package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"knsight-go/internal/hub"
	"knsight-go/internal/hub/ck"
	"knsight-go/internal/hub/dashboard"
	"knsight-go/internal/hub/kim"
	"knsight-go/internal/hub/memory"
	"knsight-go/internal/hub/skills"
	"knsight-go/internal/hub/store"
	"knsight-go/internal/hub/user"
	"knsight-go/internal/registry"
)

//go:embed chat.html
var chatHTML []byte

type ChatRequest struct {
	Message      string `json:"message"`
	RunID        string `json:"run_id"`
	SessionID    string `json:"session_id"`
	Stream       bool   `json:"stream"`
	SceneID      string `json:"scene_id,omitempty"` // scene identifier for routing and metadata
	AutoApprove  *bool  `json:"auto_approve,omitempty"`
	Model        string `json:"model,omitempty"` // user-selected model label or model_id; empty/"Knsight" = system default
	LimitProfile string `json:"limit_profile,omitempty"`
}

type ResumeRequest struct {
	SessionID string         `json:"session_id,omitempty"`
	RunID     string         `json:"run_id"`
	TargetID  string         `json:"target_id"`
	Data      any            `json:"data"`
	Targets   map[string]any `json:"targets"`
	Stream    bool           `json:"stream"`
}

type StateResponse struct {
	RunID         string `json:"run_id"`
	HasCheckpoint bool   `json:"has_checkpoint"`
}

type StreamEnvelope struct {
	Type      string           `json:"type"`
	Event     *hub.PublicEvent `json:"event,omitempty"`
	Result    *hub.RunResult   `json:"result,omitempty"`
	SessionID string           `json:"session_id,omitempty"`
}

type SubmitTaskResponse struct {
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
}

type snapshotActivity struct {
	ID        string         `json:"id"`
	AgentName string         `json:"agentName"`
	Type      string         `json:"type"`
	Content   string         `json:"content"`
	Timestamp string         `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type snapshotToolCall struct {
	Tool       string         `json:"tool"`
	Arguments  map[string]any `json:"arguments"`
	Reasoning  string         `json:"reasoning,omitempty"`
	Success    bool           `json:"success"`
	Output     string         `json:"output,omitempty"`
	AgentName  string         `json:"agentName,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
}

type snapshotState struct {
	AgentActivities []snapshotActivity `json:"agentActivities"`
	ToolCalls       []snapshotToolCall `json:"toolCalls"`
	Images          []any              `json:"images"`
	Todos           []any              `json:"todos"`
	ThinkingHistory []any              `json:"thinkingHistory"`
	ReportData      any                `json:"reportData"`
	TotalSteps      int                `json:"totalSteps"`
	TotalToolCalls  int                `json:"totalToolCalls"`
}

// loggingMiddleware logs every request with method, path, status, and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lw, r)
		u := user.FromContext(r.Context())
		log.Printf("[%s] %s user=%s %s → %d (%s)", r.Method, r.URL.Path, u.ID, r.RemoteAddr, lw.status, time.Since(start).Round(time.Millisecond))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return // prevent superfluous WriteHeader calls (e.g. SSE then error)
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// Ensure loggingResponseWriter also implements http.Flusher for SSE streaming.
func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func main() {
	configPath := flag.String("config", "configs/hub.yaml", "hub config path")
	flag.Parse()

	cfg, err := hub.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Scene support: KNSIGHT_SCENE env var overrides the agent tree with scene-specific config.
	if sceneID := os.Getenv("KNSIGHT_SCENE"); sceneID != "" {
		sceneConfigPath := fmt.Sprintf("configs/scene-%s.yaml", sceneID)
		sc, scErr := hub.LoadSceneConfig(sceneConfigPath)
		if scErr != nil {
			log.Fatalf("load scene config %q: %v", sceneConfigPath, scErr)
		}
		if applyErr := cfg.ApplyScene(sc); applyErr != nil {
			log.Fatalf("apply scene %q: %v", sceneID, applyErr)
		}
		log.Printf("scene mode: %s (config: %s, supervisor: %s, sub_agents: %d)",
			sceneID, sceneConfigPath, cfg.Supervisor.Name, len(cfg.SubAgents))
	}

	// Initialize zap logger (console + file)
	logger, err := hub.InitLogger(cfg.Log)
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)
	// From here on, standard log.Printf also writes through zap
	_ = zap.RedirectStdLog(logger)

	ctx := context.Background()

	// Create embedded registry
	regStore := registry.NewStore()
	regServer := registry.NewServer(regStore)
	stop := make(chan struct{})
	defer close(stop)
	regServer.StartCleanupLoop(5*time.Second, stop)

	// Initialize SQLite (always needed for config, filetree, etc.)
	db, err := store.NewDB(cfg.Store.DBPath)
	if err != nil {
		log.Fatalf("init sqlite: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	log.Printf("sqlite store: %s", cfg.Store.DBPath)

	// Build the SessionStore: either Redis or SQLite
	var sessionStore store.SessionStore
	var redisStore *store.RedisStore // kept for file operations (skills/memory sync)

	if cfg.Redis.ResourceName != "" {
		redisStore, err = store.NewRedisStore(cfg.Redis.ResourceName, cfg.Redis.Prefix)
		if err != nil {
			log.Printf("warning: redis init failed: %v — falling back to SQLite", err)
			redisStore = nil
		} else {
			log.Printf("redis store enabled (resource=%s, prefix=%s)", cfg.Redis.ResourceName, cfg.Redis.Prefix)
			if pingErr := redisStore.Ping(ctx); pingErr != nil {
				log.Printf("warning: redis ping failed: %v — Redis may not be functional", pingErr)
			} else {
				log.Printf("redis ping OK")
			}
			if cfg.Store.UseRedis() {
				// Startup sync: pull skills and memory files from Redis → local dirs
				if cfg.Skills.Enabled && cfg.Skills.SkillDir != "" {
					if syncErr := redisStore.SyncFilesToDir(ctx, "skills", cfg.Skills.SkillDir); syncErr != nil {
						log.Printf("warning: redis skills sync: %v", syncErr)
					}
				}
				if cfg.Memory.Enabled && cfg.Memory.WorkspaceDir != "" {
					memDir := cfg.Memory.WorkspaceDir + "/memory"
					if syncErr := redisStore.SyncFilesToDir(ctx, "memory", memDir); syncErr != nil {
						log.Printf("warning: redis memory sync: %v", syncErr)
					}
				}
			}
		}
	}

	// Final session store assignment
	if cfg.Store.UseRedis() && redisStore != nil {
		sessionStore = redisStore
		log.Printf("session store: redis")
	} else {
		sessionStore = store.NewSQLiteStore(db)
		log.Printf("session store: sqlite")
	}

	hubOpts := []hub.HubOption{hub.WithRegistryStore(regStore)}
	if redisStore != nil {
		hubOpts = append(hubOpts, hub.WithLLMRoundSink(func(ctx context.Context, record hub.LLMRoundRecord) error {
			payload, err := json.Marshal(record)
			if err != nil {
				return fmt.Errorf("marshal llm round: %w", err)
			}
			return redisStore.AppendLLMRound(ctx, record.SessionID, record.RunID, string(payload))
		}))
		log.Printf("llm round tracing enabled (redis key=%s:llm_rounds:{session_id}:{run_id}, ttl=365d)", redisStore.Prefix())
	}

	h, err := hub.NewHub(ctx, cfg, hubOpts...)
	if err != nil {
		log.Fatalf("init hub: %v", err)
	}
	defer func() {
		_ = h.Close()
	}()

	// Dashboard aggregator — always on. When cfg.SceneID is empty (普通版部署如
	// hub.prod.yaml)，aggregator 把 sceneID 当作"不过滤"，对 Redis/SQLite 中所有历史
	// session 做聚合，启动时即重建看板缓存（aggregator.Start 会冷启动跑一遍全部 range）。
	var dashAgg *dashboard.Aggregator
	sceneLabel := cfg.SceneID
	if sceneLabel == "" {
		sceneLabel = "<all>"
	}
	if cfg.Store.UseRedis() && redisStore != nil {
		dashAgg = dashboard.NewAggregatorWithRedis(
			sessionStore, cfg.SceneID,
			dashboard.NewRedisAdapter(redisStore.Client()),
			redisStore.Prefix(),
		)
		log.Printf("dashboard aggregator started for scene=%s (redis-backed)", sceneLabel)
	} else {
		dashAgg = dashboard.NewAggregator(sessionStore, cfg.SceneID)
		log.Printf("dashboard aggregator started for scene=%s (in-memory)", sceneLabel)
	}
	dashAgg.Start()
	defer dashAgg.Stop()

	mux := http.NewServeMux()

	// Health check (kcsize probes /api/healthz)
	healthz := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/api/healthz", healthz)

	// User info endpoint — enriches with Halo profile (display name, avatar)
	profileCache := user.NewProfileCache(10*time.Minute, cfg.ProfileToken)
	mux.HandleFunc("/v1/user/me", func(w http.ResponseWriter, r *http.Request) {
		u := user.FromContext(r.Context())
		profile := profileCache.GetProfile(u.ID, r)
		writeJSON(w, profile)
	})

	// Registry routes (embedded)
	mux.HandleFunc("/v1/registry/register", regServer.HandleRegister)
	mux.HandleFunc("/v1/registry/heartbeat", regServer.HandleHeartbeat)
	mux.HandleFunc("/v1/registry/agents", regServer.HandleList)
	mux.HandleFunc("/v1/registry/agents/", regServer.HandleGet)

	// Skills routes
	if cfg.Skills.Enabled {
		skillsLoader := skills.NewLoader(cfg.Skills.SkillDir)
		if err := skillsLoader.Load(); err != nil {
			log.Printf("warning: skills load: %v", err)
		}
		skillsHandler := skills.NewHandler(skillsLoader)
		if redisStore != nil {
			skillsHandler.SetWriteHook(func(scope, name, relPath, content string) error {
				if err := redisStore.SetFile(ctx, "skills", relPath, content); err != nil {
					log.Printf("redis skills write %s: %v", relPath, err)
					return err
				}
				return nil
			})
		}
		skillsHandler.RegisterRoutes(mux)
		log.Printf("skills routes mounted (%d skills)", len(skillsLoader.ListSkills()))
	}

	// Memory routes
	if cfg.Memory.Enabled {
		memStore := h.MemoryStore()
		if memStore != nil {
			if redisStore != nil {
				memStore.SetWriteHook(func(relPath, content string) {
					go func() {
						if err := redisStore.SetFile(ctx, "memory", relPath, content); err != nil {
							log.Printf("redis memory write %s: %v", relPath, err)
						}
					}()
				})
			}
			// resolveMemStore returns either the shared store or a user-scoped store
			// based on the "scope" query parameter.
			resolveMemStore := func(r *http.Request) *memory.MemoryStore {
				scope := r.URL.Query().Get("scope")
				if uid, ok := strings.CutPrefix(scope, "user/"); ok {
					u := user.FromContext(r.Context())
					if uid == u.ID {
						return memStore.ForUser(uid)
					}
					return nil // unauthorized
				}
				return memStore
			}

			mux.HandleFunc("/v1/memory", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					ms := resolveMemStore(r)
					if ms == nil {
						http.Error(w, "unauthorized", http.StatusForbidden)
						return
					}
					ctx := ms.GetMemoryContext()
					longTerm, _ := ms.ReadLongTerm()
					today, _ := ms.ReadToday()
					writeJSON(w, map[string]any{
						"long_term":      longTerm,
						"today":          today,
						"memory_context": ctx,
					})
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/v1/memory/long-term", func(w http.ResponseWriter, r *http.Request) {
				ms := resolveMemStore(r)
				if ms == nil {
					http.Error(w, "unauthorized", http.StatusForbidden)
					return
				}
				switch r.Method {
				case http.MethodGet:
					content, err := ms.ReadLongTerm()
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					writeJSON(w, map[string]string{"content": content})
				case http.MethodPut:
					var body struct {
						Content string `json:"content"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := ms.WriteLongTerm(body.Content); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					writeJSON(w, map[string]string{"status": "ok"})
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/v1/memory/today", func(w http.ResponseWriter, r *http.Request) {
				ms := resolveMemStore(r)
				if ms == nil {
					http.Error(w, "unauthorized", http.StatusForbidden)
					return
				}
				switch r.Method {
				case http.MethodGet:
					content, err := ms.ReadToday()
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					writeJSON(w, map[string]string{"content": content})
				case http.MethodPost:
					var body struct {
						Content string `json:"content"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if err := ms.AppendToday(body.Content); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					writeJSON(w, map[string]string{"status": "ok"})
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})
			log.Printf("memory routes mounted (workspace=%s)", cfg.Memory.WorkspaceDir)
		}
	}

	// ClickHouse gateway routes — mounted unconditionally so callers get a
	// clear 503 when the feature is disabled. Auth is identity-passthrough
	// via Ks-Auth-* headers configured in cfg.CK.
	{
		var ckClient *ck.Client
		ckEnabled := cfg.CK.Enabled
		if ckEnabled {
			c, cerr := ck.New(ck.Config{
				GatewayURL:      cfg.CK.GatewayURL,
				Token:           cfg.CK.Token,
				User:            cfg.CK.User,
				Principal:       cfg.CK.Principal,
				AuthType:        cfg.CK.AuthType,
				DefaultTimeout:  time.Duration(cfg.CK.TimeoutSec) * time.Second,
				DefaultLimit:    cfg.CK.DefaultLimit,
				MaxRowsReturned: cfg.CK.MaxRows,
			})
			if cerr != nil {
				log.Printf("warning: ck client init failed: %v — endpoint will return 503", cerr)
				ckEnabled = false
			} else {
				ckClient = c
				log.Printf("ck routes mounted (gateway=%s user=%s default_limit=%d)",
					cfg.CK.GatewayURL, cfg.CK.User, cfg.CK.DefaultLimit)
			}
		} else {
			log.Printf("ck routes mounted (disabled — set ck.enabled=true or KNSIGHT_CK_ENABLED=true)")
		}
		ck.NewHandler(ckClient, ckEnabled).RegisterRoutes(mux)
	}

	// Session routes — uses the unified SessionStore
	sessionAPI := store.NewSessionAPI(sessionStore)
	mux.HandleFunc("/v1/sessions", sessionAPI.HandleSessions())
	mux.HandleFunc("/v1/sessions/shared/", sessionAPI.HandleSharedSession())
	mux.HandleFunc("/v1/sessions/", sessionAPI.HandleSessionByID())
	log.Printf("session routes mounted")

	// Dashboard API — aggregated metrics for scene monitoring
	mux.HandleFunc("/v1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rangeKey := r.URL.Query().Get("range")
		if rangeKey == "" {
			rangeKey = "24h"
		}
		// Validate range
		validRange := false
		for _, r := range dashboard.Ranges {
			if rangeKey == r {
				validRange = true
				break
			}
		}
		if !validRange {
			http.Error(w, `{"error":"invalid range, use: 1h, 24h, 7d, 30d, all"}`, http.StatusBadRequest)
			return
		}

		if dashAgg == nil {
			// Not in scene mode — return a minimal empty response
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"range":"` + rangeKey + `","session_count":0,"last_updated":"0001-01-01T00:00:00Z"}`))
			return
		}

		data := dashAgg.Get(rangeKey)
		if data == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"range":"` + rangeKey + `","session_count":0,"last_updated":"0001-01-01T00:00:00Z"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(data)
	})

	// Kim feedback route — sends session details to Kim group chat
	var kimClient *kim.Client
	if cfg.Kim.Enabled && cfg.Kim.APIKey != "" {
		kimClient = kim.NewClient(cfg.Kim.APIKey, cfg.Kim.BaseURL)
		hub.SetErrorNotifier(kimClient.SendMarkdown)
		log.Printf("kim notifier enabled (error alerts ON)")
	}
	mux.HandleFunc("/v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if kimClient == nil {
			http.Error(w, "kim not configured", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			SessionID string `json:"session_id"`
			Comment   string `json:"comment"`
			Rating    string `json:"rating"` // "good" or "bad"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.SessionID == "" {
			http.Error(w, "session_id is required", http.StatusBadRequest)
			return
		}

		u := user.FromContext(r.Context())

		// Collect session info
		sess, err := sessionStore.GetSession(r.Context(), req.SessionID)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		msgs, _ := sessionStore.GetMessages(r.Context(), req.SessionID)
		events, _ := sessionStore.GetSessionEvents(r.Context(), req.SessionID)

		// Ensure share link exists — auto-generate if missing
		shareURL := ""
		if sess.ShareToken != nil && *sess.ShareToken != "" {
			shareURL = "/shared/" + *sess.ShareToken
		} else {
			token := uuid.NewString()
			if err := sessionStore.SetShareToken(r.Context(), req.SessionID, token); err == nil {
				sess.ShareToken = &token
				_ = sessionStore.UpdateSession(r.Context(), sess)
				shareURL = "/shared/" + token
			}
		}

		// Extract used agents from events (deduplicated)
		usedAgentSet := make(map[string]bool)
		for _, ev := range events {
			if ev.AgentName != "" {
				usedAgentSet[ev.AgentName] = true
			}
		}
		var usedAgents []string
		for name := range usedAgentSet {
			usedAgents = append(usedAgents, name)
		}

		// Extract used skills from messages (look for skill references in assistant content)
		usedSkillSet := make(map[string]bool)
		if h.SkillsLoader() != nil {
			allSkills := h.SkillsLoader().ListSkills()
			for _, m := range msgs {
				if m.Role == "assistant" {
					for _, s := range allSkills {
						if strings.Contains(m.Content, s.Name) {
							usedSkillSet[s.Name] = true
						}
					}
				}
			}
		}
		var usedSkills []string
		for name := range usedSkillSet {
			usedSkills = append(usedSkills, name)
		}

		// Redis keys
		redisKeys := ""
		if cfg.Store.UseRedis() {
			prefix := cfg.Redis.Prefix
			redisKeys = fmt.Sprintf(
				"`%s:session:%s`\n`%s:snapshot:%s`\n`%s:messages:%s`\n`%s:events:%s`",
				prefix, req.SessionID, prefix, req.SessionID,
				prefix, req.SessionID, prefix, req.SessionID,
			)
		}

		// Message summary
		msgSummary := fmt.Sprintf("%d 条消息", len(msgs))
		firstUserMsg := ""
		for _, m := range msgs {
			if m.Role == "user" {
				firstUserMsg = m.Content
				if len(firstUserMsg) > 200 {
					firstUserMsg = firstUserMsg[:197] + "..."
				}
				break
			}
		}

		// Rating: default "不好" if user didn't explicitly choose
		ratingStr := "不好"
		if req.Rating == "good" {
			ratingStr = "好"
		}

		// Build markdown message (Chinese)
		now := time.Now().Format("2006-01-02 15:04:05")
		var sb strings.Builder
		sb.WriteString("# <font color=\"#1890ff\">**KNsight 用户使用反馈**</font>\n\n")
		sb.WriteString(fmt.Sprintf("**评价**: %s\n\n", ratingStr))
		sb.WriteString(fmt.Sprintf("**用户**: %s\n\n", u.ID))
		sb.WriteString(fmt.Sprintf("**时间**: %s\n\n", now))
		sb.WriteString(fmt.Sprintf("**会话 ID**: `%s`\n\n", req.SessionID))
		sb.WriteString(fmt.Sprintf("**Agent**: %s\n\n", sess.AgentType))
		sb.WriteString(fmt.Sprintf("**消息数**: %s\n\n", msgSummary))
		if firstUserMsg != "" {
			sb.WriteString(fmt.Sprintf("**用户提问**: %s\n\n", firstUserMsg))
		}
		if len(usedAgents) > 0 {
			sb.WriteString(fmt.Sprintf("**使用的 Agent**: %s\n\n", strings.Join(usedAgents, ", ")))
		}
		if len(usedSkills) > 0 {
			sb.WriteString(fmt.Sprintf("**使用的 Skill**: %s\n\n", strings.Join(usedSkills, ", ")))
		}
		if shareURL != "" {
			sb.WriteString(fmt.Sprintf("**对话链接**: %s\n\n", shareURL))
		}
		if redisKeys != "" {
			sb.WriteString(fmt.Sprintf("**Redis Keys**:\n%s\n\n", redisKeys))
		}
		if req.Comment != "" {
			sb.WriteString(fmt.Sprintf("**备注**: %s\n", req.Comment))
		}

		if err := kimClient.SendMarkdown(sb.String()); err != nil {
			log.Printf("[kim] send feedback failed: %v", err)
			http.Error(w, "failed to send feedback: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[kim] feedback sent user=%s session=%s rating=%s", u.ID, req.SessionID, req.Rating)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// Debug endpoint — view assembled prompts
	mux.HandleFunc("/v1/debug/prompts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		u := user.FromContext(r.Context())
		writeJSON(w, h.DebugPrompts(u.ID))
	})

	// Admin endpoint — list all sessions across all users
	mux.HandleFunc("/v1/debug/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 100
		}
		sessions, err := sessionStore.ListSessions(r.Context(), "", limit, offset, "", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, sessions)
	})

	// File tree routes (always SQLite)
	fileTreeAPI := store.NewFileTreeAPI(db, map[string]string{
		"skills": cfg.Skills.SkillDir,
		"memory": cfg.Memory.WorkspaceDir,
	})
	// Mirror filetree writes/deletes to Redis so user edits survive container
	// restarts (where the rootfs is reset to the image layer and start.sh re-seeds
	// bundled-skills). Redis is the source of truth for SyncFilesToDir on boot.
	if redisStore != nil {
		categoryFor := func(root string) string {
			switch root {
			case "skills":
				return "skills"
			case "memory":
				return "memory"
			default:
				return ""
			}
		}
		fileTreeAPI.OnWrite = func(root, relPath, content string) {
			cat := categoryFor(root)
			if cat == "" {
				return
			}
			if err := redisStore.SetFile(ctx, cat, relPath, content); err != nil {
				log.Printf("warning: redis mirror write %s/%s: %v", cat, relPath, err)
			}
		}
		fileTreeAPI.OnDelete = func(root, relPath string) {
			cat := categoryFor(root)
			if cat == "" {
				return
			}
			// Best-effort: handle both single file and directory prefixes.
			if err := redisStore.DeleteFilePrefix(ctx, cat, relPath); err != nil {
				log.Printf("warning: redis mirror delete %s/%s: %v", cat, relPath, err)
			}
		}
	}
	mux.HandleFunc("/v1/filetree", fileTreeAPI.Handle())
	mux.HandleFunc("/v1/filetree/", fileTreeAPI.Handle())
	if cfg.Skills.Enabled {
		if err := store.SyncFS(db, "skills", cfg.Skills.SkillDir); err != nil {
			log.Printf("warning: skills sync: %v", err)
		}
	}
	if cfg.Memory.Enabled && cfg.Memory.WorkspaceDir != "" {
		if err := store.SyncFS(db, "memory", cfg.Memory.WorkspaceDir); err != nil {
			log.Printf("warning: memory sync: %v", err)
		}
	}
	log.Printf("filetree routes mounted")

	// Config routes — DB-backed so settings persist across restarts
	configAPI := hub.NewConfigAPIWithStore(*configPath, db)
	configAPI.SeedDBFromConfig(&cfg)
	mux.HandleFunc("/v1/config", configAPI.HandleConfig())
	mux.HandleFunc("/v1/config/", configAPI.HandleConfigSubpath())

	// Register config watcher: version-gated hot-reload for memory/skills/agents.
	h.SetConfigWatcher(configAPI)
	log.Printf("config routes mounted")

	// /v1/models — return user-selectable model list
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type modelOption struct {
			Label   string `json:"label"`
			ModelID string `json:"model_id"`
		}
		models := h.AvailableModels()
		opts := make([]modelOption, 0, len(models))
		for _, m := range models {
			opts = append(opts, modelOption{Label: m.Label, ModelID: m.ModelID})
		}
		writeJSON(w, map[string]any{"models": opts})
	})
	mux.HandleFunc("/v1/run-limit-profiles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{"profiles": h.AvailableRunLimitProfiles()})
	})

	// /v1/chat — full RunResult format (events, interrupts, output)
	chatHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runID := req.RunID
		if runID == "" {
			runID = hub.NewRunID()
		}

		u := user.FromContext(r.Context())
		log.Printf("[chat] run_id=%s user=%s stream=%v model=%q message=%q", runID, u.ID, req.Stream, req.Model, truncate(req.Message, 100))

		// Version-gated hot-reload: ~1 ns on the happy path (no change).
		h.CheckAndReloadIfChanged()

		// Get or create session
		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = uuid.NewString()
		}
		// Use scene_id from request body, or fall back to Hub's configured sceneID
		sceneID := req.SceneID
		if sceneID == "" {
			sceneID = h.SceneID()
		}
		ensureSession(sessionStore, sessionID, u.ID, req.Message, sceneID)
		stampSessionModel(sessionStore, h, sessionID, req.Model)

		// Load conversation history for follow-up context
		prevMsgs, _ := sessionStore.GetMessages(r.Context(), sessionID)
		var history []hub.ConversationMessage
		for _, m := range prevMsgs {
			history = append(history, hub.ConversationMessage{Role: m.Role, Content: m.Content})
		}

		// Save user message before processing
		_ = sessionStore.AddMessage(r.Context(), &store.SessionMessage{
			SessionID: sessionID,
			Role:      "user",
			Content:   req.Message,
		})

		chatStart := time.Now()
		// Use background context for agent execution — detached from HTTP request.
		// This prevents client disconnect (page refresh, navigation) from killing
		// the agent mid-execution. Only idle timeout or explicit cancel stops it.
		ctx, cancel := context.WithCancel(context.Background())
		// Propagate user info from request context
		ctx = user.WithContext(ctx, u)
		var contextCompactionEvents chan hub.ContextCompactionEvent
		if req.Stream {
			contextCompactionEvents = make(chan hub.ContextCompactionEvent, 16)
			ctx = hub.WithContextCompactionCallback(ctx, func(event hub.ContextCompactionEvent) {
				select {
				case contextCompactionEvents <- event:
				default:
					log.Printf("[chat] run_id=%s context compaction progress channel full; dropping status=%s", runID, event.Status)
				}
			})
		}
		if req.AutoApprove != nil {
			ctx = hub.WithAutoApprove(ctx, *req.AutoApprove)
		}
		defer cancel()

		// Cancel agent if client disconnects (abort button / page close)
		go func() {
			<-r.Context().Done()
			// Give agent a grace period to finish current step
			time.Sleep(5 * time.Second)
			cancel()
		}()

		if req.Stream {
			// Idle timer: 10 minutes — if no event for 10min, cancel
			timer := time.AfterFunc(10*time.Minute, func() {
				log.Printf("[chat] run_id=%s idle timeout (10min) after %s total, cancelling", runID, time.Since(chatStart).Round(time.Second))
				cancel()
			})
			iter := h.RunIterWithSessionOptions(ctx, sessionID, runID, req.Message, u.ID, history, req.Model, req.LimitProfile)
			result := streamEvents(w, iter, runID, timer, sessionID, contextCompactionEvents)
			timer.Stop()
			// Retry once if ChatModel error with no output (likely LLM API failure)
			if result != nil && result.Output == "" && len(result.Events) <= 1 {
				log.Printf("[chat] run_id=%s empty result, retrying once...", runID)
				retryID := hub.NewRunID()
				timer2 := time.AfterFunc(10*time.Minute, cancel)
				iter2 := h.RunIterWithSessionOptions(ctx, sessionID, retryID, req.Message, u.ID, history, req.Model, req.LimitProfile)
				result = streamEvents(w, iter2, retryID, timer2, sessionID, contextCompactionEvents)
				timer2.Stop()
			}
			if result != nil {
				saveSessionResult(sessionStore, sessionID, result)
			}
			return
		}

		// Non-streaming: 10 minute timeout
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 10*time.Minute)
		defer timeoutCancel()

		result, err := h.RunWithSessionOptions(timeoutCtx, sessionID, runID, req.Message, u.ID, history, req.Model, req.LimitProfile)
		if err != nil {
			log.Printf("[chat] run_id=%s error: %v", runID, err)
			hub.NotifyError("chat:"+runID, hub.ErrorParams{
				Component: "Chat",
				SessionID: sessionID,
				RunID:     runID,
				UserID:    u.ID,
				Input:     truncate(req.Message, 200),
				Error:     err.Error(),
			})
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i, ev := range result.Events {
			var msg *schema.Message
			if ev.Output != nil {
				msg = ev.Output.Message
			}
			logEventDetail(runID, i+1, ev, msg)
		}
		log.Printf("[chat] run_id=%s done events=%d output_len=%d interrupts=%d",
			runID, len(result.Events), len(result.Output), len(result.Interrupts))
		result.SessionID = sessionID
		result.Output = stripKnsightTags(result.Output)
		saveSessionResult(sessionStore, sessionID, result)
		writeJSON(w, result)
	}
	mux.HandleFunc("/v1/chat", chatHandler)

	// /v1/submit-task — enqueue an async diagnosis and return its session id.
	submitTaskHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Message) == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}

		runID := req.RunID
		if runID == "" {
			runID = hub.NewRunID()
		}
		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = uuid.NewString()
		}

		u := user.FromContext(r.Context())
		sceneID := req.SceneID
		if sceneID == "" {
			sceneID = h.SceneID()
		}
		log.Printf("[submit-task] run_id=%s session=%s user=%s message=%q", runID, sessionID, u.ID, truncate(req.Message, 100))

		h.CheckAndReloadIfChanged()
		ensureSession(sessionStore, sessionID, u.ID, req.Message, sceneID)
		stampSessionModel(sessionStore, h, sessionID, req.Model)

		prevMsgs, _ := sessionStore.GetMessages(r.Context(), sessionID)
		history := make([]hub.ConversationMessage, 0, len(prevMsgs))
		for _, m := range prevMsgs {
			history = append(history, hub.ConversationMessage{Role: m.Role, Content: m.Content})
		}
		_ = sessionStore.AddMessage(r.Context(), &store.SessionMessage{
			SessionID: sessionID,
			Role:      "user",
			Content:   req.Message,
		})
		markSessionTaskStatus(sessionStore, sessionID, "running", "")

		go runSubmittedTask(h, sessionStore, req, runID, sessionID, u, history)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, SubmitTaskResponse{
			SessionID: sessionID,
			RunID:     runID,
			Status:    "submitted",
		})
	}
	mux.HandleFunc("/v1/submit-task", submitTaskHandler)

	// /v1/chat/completions — OpenAI Chat Completions compatible format
	chatCompletionsHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runID := req.RunID
		if runID == "" {
			runID = hub.NewRunID()
		}
		model := cfg.LLM.Model

		u := user.FromContext(r.Context())
		log.Printf("[completions] run_id=%s user=%s stream=%v message=%q", runID, u.ID, req.Stream, truncate(req.Message, 100))

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		if req.AutoApprove != nil {
			ctx = hub.WithAutoApprove(ctx, *req.AutoApprove)
		}

		if req.Stream {
			iter := h.RunIter(ctx, runID, req.Message, u.ID, nil)
			streamOpenAIChunks(w, iter, runID, model)
			return
		}

		result, err := h.Run(ctx, runID, req.Message, u.ID, nil)
		if err != nil {
			log.Printf("[completions] run_id=%s error: %v", runID, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[completions] run_id=%s done events=%d output_len=%d interrupts=%d",
			runID, len(result.Events), len(result.Output), len(result.Interrupts))
		writeJSON(w, hub.RunResultToOpenAI(result, model))
	}
	mux.HandleFunc("/v1/chat/completions", chatCompletionsHandler)

	// Resume endpoint — retries up to 3 times on transfer_to_agent error
	mux.HandleFunc("/v1/workflow/resume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req ResumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.RunID == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}
		targets := req.Targets
		if len(targets) == 0 && req.TargetID != "" {
			targets = map[string]any{req.TargetID: req.Data}
		}

		log.Printf("[resume] run_id=%s targets=%d", req.RunID, len(targets))

		// Detach from HTTP context — client disconnect won't kill the agent
		u := user.FromContext(r.Context())
		ctx, cancel := context.WithCancel(context.Background())
		ctx = user.WithContext(ctx, u)
		var contextCompactionEvents chan hub.ContextCompactionEvent
		if req.Stream {
			contextCompactionEvents = make(chan hub.ContextCompactionEvent, 16)
			ctx = hub.WithContextCompactionCallback(ctx, func(event hub.ContextCompactionEvent) {
				select {
				case contextCompactionEvents <- event:
				default:
					log.Printf("[resume] run_id=%s context compaction progress channel full; dropping status=%s", req.RunID, event.Status)
				}
			})
		}
		defer cancel()

		go func() {
			<-r.Context().Done()
			time.Sleep(5 * time.Second)
			cancel()
		}()

		const maxRetries = 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			iter, err := h.ResumeIterWithSession(ctx, req.SessionID, req.RunID, targets)
			if err != nil {
				log.Printf("[resume] run_id=%s attempt=%d init error: %v", req.RunID, attempt, err)
				if attempt < maxRetries && isTransferToAgentError(err) {
					log.Printf("[resume] run_id=%s retrying (%d/%d)...", req.RunID, attempt, maxRetries)
					time.Sleep(500 * time.Millisecond)
					continue
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if req.Stream {
				timer := time.AfterFunc(10*time.Minute, cancel)
				result := streamEvents(w, iter, req.RunID, timer, "", contextCompactionEvents)
				timer.Stop()
				if result != nil && result.TransferError {
					log.Printf("[resume] run_id=%s stream transfer_to_agent error, attempt=%d/%d", req.RunID, attempt, maxRetries)
					if attempt < maxRetries {
						time.Sleep(500 * time.Millisecond)
						continue
					}
					// Last attempt: return partial result (already streamed events)
				}
				return
			}

			result, err := collectFromIter(iter, req.RunID)
			if err != nil {
				if attempt < maxRetries && isTransferToAgentError(err) {
					log.Printf("[resume] run_id=%s transfer_to_agent error, retrying (%d/%d)...", req.RunID, attempt, maxRetries)
					time.Sleep(500 * time.Millisecond)
					continue
				}
				log.Printf("[resume] run_id=%s error: %v", req.RunID, err)
				hub.NotifyError("resume:"+req.RunID, hub.ErrorParams{
					Component: "Resume",
					RunID:     req.RunID,
					Error:     err.Error(),
				})
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if result.TransferError && attempt < maxRetries {
				log.Printf("[resume] run_id=%s transfer_to_agent error, retrying (%d/%d)...", req.RunID, attempt, maxRetries)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Printf("[resume] run_id=%s done events=%d attempt=%d", req.RunID, len(result.Events), attempt)
			writeJSON(w, result)
			return
		}
	})

	// State endpoint
	mux.HandleFunc("/v1/workflow/state/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		runID := strings.TrimPrefix(r.URL.Path, "/v1/workflow/state/")
		if runID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeJSON(w, StateResponse{RunID: runID, HasCheckpoint: h.HasCheckpoint(runID)})
	})

	// Status endpoint - show all subsystem status
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		status := map[string]any{
			"sandbox":      cfg.Sandbox.Enabled,
			"memory":       cfg.Memory.Enabled,
			"skills":       cfg.Skills.Enabled,
			"tools_mcps":   len(cfg.Tools.MCPs),
			"tools_agents": len(cfg.Tools.Agents),
			"sanitizer":    hub.GetSanitizerStats(),
		}
		if cfg.Skills.Enabled && h.SkillsLoader() != nil {
			status["skills_count"] = len(h.SkillsLoader().ListSkills())
			status["skills_scopes"] = h.SkillsLoader().GetScopes()
		}
		if cfg.Memory.Enabled && h.MemoryStore() != nil {
			longTerm, _ := h.MemoryStore().ReadLongTerm()
			status["memory_has_long_term"] = len(longTerm) > 0
		}
		status["registry_agents"] = len(regStore.List())
		writeJSON(w, status)
	})

	// Serve frontend static files (Next.js export) or fallback to embedded chat.html
	frontendDir := "frontend/out"
	if info, err := os.Stat(frontendDir); err == nil && info.IsDir() {
		fs := http.FileServer(http.Dir(frontendDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Try serving static file first
			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}
			fullPath := frontendDir + path
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
			// For Next.js client-side routing, try path.html
			htmlPath := frontendDir + path + ".html"
			if _, err := os.Stat(htmlPath); err == nil {
				http.ServeFile(w, r, htmlPath)
				return
			}
			// For dynamic Next.js routes, look for __placeholder__.html in parent dir.
			segments := strings.Split(strings.Trim(path, "/"), "/")
			if len(segments) >= 2 {
				placeholderPath := frontendDir + "/" + segments[0] + "/__placeholder__.html"
				if _, err := os.Stat(placeholderPath); err == nil {
					http.ServeFile(w, r, placeholderPath)
					return
				}
			}
			// Fallback to index.html for SPA routing
			http.ServeFile(w, r, frontendDir+"/index.html")
		})
		log.Printf("frontend:  %s", frontendDir)
	} else {
		// Fallback: serve embedded chat.html
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(chatHTML)
		})
		log.Printf("frontend:  embedded (chat.html)")
	}

	log.Printf("hub listening on %s", cfg.ListenAddr)
	log.Printf("  registry: embedded")
	log.Printf("  sandbox:  %v", cfg.Sandbox.Enabled)
	log.Printf("  memory:   %v (workspace=%s)", cfg.Memory.Enabled, cfg.Memory.WorkspaceDir)
	log.Printf("  skills:   %v (dir=%s)", cfg.Skills.Enabled, cfg.Skills.SkillDir)
	log.Printf("  tools:    %d mcps, %d agents", len(cfg.Tools.MCPs), len(cfg.Tools.Agents))
	log.Printf("  llm:      %s (model=%s)", cfg.LLM.BaseURL, cfg.LLM.Model)

	// Auth wiring:
	//   - master toggle:  cfg.Auth.Enabled  (env KNSIGHT_AUTH_ENABLED)
	//   - backend select: cfg.Auth.Mode     (env KNSIGHT_AUTH_MODE: cookie|accessproxy|auto|disabled)
	//   - legacy fallback: cfg.Auth.SSORequired / KNSIGHT_SSO_REQUIRED → cookie mode
	ssoRequired := cfg.Auth.SSORequired || user.IsSSORequired()
	ssoURL := cfg.Auth.SSOURL
	if ssoURL == "" {
		ssoURL = user.GetSSOURL()
	}

	authEnabled := user.IsAuthEnabled(cfg.Auth.Enabled)
	mode := user.ResolveAuthMode(cfg.Auth.Mode, ssoRequired)
	if !authEnabled {
		mode = user.AuthModeDisabled
	}
	serviceAuth := user.ServiceAuthConfigFromEnv()
	user.LogServiceAuthConfig(serviceAuth)
	inboundJWTAuth := user.InboundJWTConfigFromEnv()
	user.LogInboundJWTConfig(inboundJWTAuth)

	loggedMux := loggingMiddleware(mux)
	var handler http.Handler

	switch mode {
	case user.AuthModeAccessProxy, user.AuthModeAuto:
		apClient, apErr := user.NewAccessProxyClient(user.AccessProxyConfig{
			JwksURL:          cfg.Auth.AccessProxy.JwksURL,
			PublicHost:       cfg.Auth.AccessProxy.PublicHost,
			VerifyIss:        cfg.Auth.AccessProxy.VerifyIss,
			TrustedHosts:     cfg.Auth.AccessProxy.TrustedHosts,
			TrustedUpstreams: cfg.Auth.AccessProxy.TrustedUpstreams,
		})
		if apErr != nil {
			log.Fatalf("AccessProxy SDK init failed: %v", apErr)
		}
		defer apClient.Close()
		log.Printf("auth: mode=%s (jwks=%s domain=%s verify_iss=%v trusted_hosts=%v trusted_upstreams=%v)",
			mode,
			firstNonEmpty(cfg.Auth.AccessProxy.JwksURL, user.DefaultJwksURL),
			cfg.Auth.AccessProxy.PublicHost,
			cfg.Auth.AccessProxy.VerifyIss,
			cfg.Auth.AccessProxy.TrustedHosts,
			cfg.Auth.AccessProxy.TrustedUpstreams,
		)
		if mode == user.AuthModeAuto {
			// AP first; if missing/invalid, fall through to cookie+SSO chain.
			handler = user.AccessProxyMiddleware(apClient, profileCache, true,
				user.SSOMiddleware(ssoRequired, ssoURL, loggedMux))
		} else {
			handler = user.AccessProxyMiddleware(apClient, profileCache, false, loggedMux)
		}
	case user.AuthModeCookie:
		log.Printf("auth: mode=cookie sso_required=%v sso_url=%s", ssoRequired, ssoURL)
		handler = user.SSOMiddleware(ssoRequired, ssoURL, loggedMux)
	default: // AuthModeDisabled
		log.Printf("auth: mode=disabled (all requests pass as visitor)")
		handler = user.Middleware(loggedMux)
	}
	handler = user.ServiceAuthMiddleware(serviceAuth, loggedMux, handler)
	handler = user.InboundJWTMiddleware(inboundJWTAuth, loggedMux, handler)

	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		log.Fatalf("hub server error: %v", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ensureSession gets or creates a session via the SessionStore.
// When sceneID is non-empty, it is stored in session metadata for dashboard filtering.
func ensureSession(s store.SessionStore, sessionID, userID, firstMessage, sceneID string) {
	if _, err := s.GetSession(context.Background(), sessionID); err == nil {
		return // already exists
	}
	title := truncateRunes(firstMessage, 50)
	agentType := "insight"
	metadata := ""
	if sceneID != "" {
		agentType = "scene/" + sceneID
		metadata = fmt.Sprintf(`{"scene_id":%q}`, sceneID)
	}
	_ = s.CreateSession(context.Background(), &store.Session{
		ID:        sessionID,
		Title:     title,
		AgentType: agentType,
		Metadata:  metadata,
		UserID:    userID,
	})
}

func stampSessionModel(s store.SessionStore, h *hub.Hub, sessionID, requestedModel string) {
	sess, err := s.GetSession(context.Background(), sessionID)
	if err != nil {
		return
	}
	var meta map[string]any
	if sess.Metadata != "" {
		_ = json.Unmarshal([]byte(sess.Metadata), &meta)
	}
	if meta == nil {
		meta = make(map[string]any)
	}

	requested := strings.TrimSpace(requestedModel)
	if requested == "" {
		requested = "Knsight"
	}
	label := requested
	modelID := h.DefaultModelID()
	if sel := h.ResolveUserModel(requestedModel); sel != nil {
		label = sel.Label
		if label == "" {
			label = sel.ModelID
		}
		modelID = sel.ModelID
	}
	meta["requested_model"] = requested
	meta["model_label"] = label
	meta["model_id"] = modelID
	meta["effective_model"] = modelID
	meta["model_recorded_at"] = time.Now().Format(time.RFC3339)

	if b, err := json.Marshal(meta); err == nil {
		sess.Metadata = string(b)
		_ = s.UpdateSession(context.Background(), sess)
	}
}

// saveSessionResult saves the assistant output and events via the SessionStore.
// Saves supervisor + sub-agent outputs as plain text messages (no tool_use IDs).
func saveSessionResult(s store.SessionStore, sessionID string, result *hub.RunResult) {
	if result == nil {
		return
	}
	// Save agent outputs + thinking as plain text context (no tool_use IDs)
	seen := make(map[string]bool)
	for _, ev := range result.Events {
		if ev.Output == nil || ev.Output.Message == nil || ev.AgentName == "" {
			continue
		}
		msg := ev.Output.Message

		// Save thinking/reasoning content
		if msg.ReasoningContent != "" && len(msg.ReasoningContent) >= 20 {
			thinkKey := "think:" + msg.ReasoningContent[:min(len(msg.ReasoningContent), 100)]
			if !seen[thinkKey] {
				seen[thinkKey] = true
				thinking := msg.ReasoningContent
				if len(thinking) > 2000 {
					thinking = thinking[:1997] + "..."
				}
				_ = s.AddMessage(context.Background(), &store.SessionMessage{
					SessionID: sessionID,
					Role:      "assistant",
					Content:   fmt.Sprintf("[%s thinking]\n%s", ev.AgentName, thinking),
				})
			}
		}

		// Save text outputs, skip tool calls/results
		if msg.Content == "" || msg.ToolCallID != "" || len(msg.ToolCalls) > 0 {
			continue
		}
		if len(msg.Content) < 20 {
			continue
		}
		key := "out:" + msg.Content[:min(len(msg.Content), 100)]
		if seen[key] {
			continue
		}
		seen[key] = true
		content := msg.Content
		if len(content) > 2000 {
			content = content[:1997] + "..."
		}
		_ = s.AddMessage(context.Background(), &store.SessionMessage{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   fmt.Sprintf("[%s]\n%s", ev.AgentName, content),
		})
	}
	// Save final supervisor output
	if result.Output != "" {
		_ = s.AddMessage(context.Background(), &store.SessionMessage{
			SessionID: sessionID,
			Role:      "assistant",
			// 标签块只用于元数据归档，从持久化的消息正文里去掉避免 /debug、详情页和
			// 分享链接里渲染成可见的 HTML 注释噪声。
			Content: stripKnsightTags(result.Output),
		})
	}
	if len(result.Events) > 0 {
		events := make([]store.SessionEvent, len(result.Events))
		for i, ev := range result.Events {
			data, _ := json.Marshal(ev)
			runPath, _ := json.Marshal(ev.RunPath)
			events[i] = store.SessionEvent{
				SessionID:  sessionID,
				EventIndex: i,
				AgentName:  ev.AgentName,
				RunPath:    string(runPath),
				EventData:  string(data),
			}
		}
		_ = s.AddSessionEvents(context.Background(), events)
		saveSynthesizedSnapshot(s, sessionID, result)
	}
	// Touch session updated_at + enrich metadata with diagnostic fields from Supervisor output
	// 看板需要 token 数据；普通版 InsightSupervisor 输出是 markdown 不是 JSON，所以
	// enrichSessionMetadata 通常不写任何字段——这里独立把 token usage 写入 metadata，
	// 让 dashboard 的 TOKENS / Token Top Users 能正确累积。
	sess, err := s.GetSession(context.Background(), sessionID)
	if err == nil {
		if result.Output != "" {
			enrichSessionMetadata(sess, result.Output)
			enrichSessionTags(sess, result.Output)
		}
		enrichSessionTokens(sess, result)
		enrichSessionStatus(sess, result)
		_ = s.UpdateSession(context.Background(), sess)
	} else {
		_ = s.UpdateSession(context.Background(), &store.Session{ID: sessionID})
	}
}

func saveSynthesizedSnapshot(s store.SessionStore, sessionID string, result *hub.RunResult) {
	snapshot := synthesizeSnapshotFromRunResult(result)
	if len(snapshot.AgentActivities) == 0 && len(snapshot.ToolCalls) == 0 && len(snapshot.Images) == 0 {
		return
	}
	if b, err := json.Marshal(snapshot); err == nil {
		_ = s.SaveSnapshot(context.Background(), sessionID, string(b))
	}
}

func synthesizeSnapshotFromRunResult(result *hub.RunResult) snapshotState {
	snapshot := snapshotState{
		AgentActivities: []snapshotActivity{},
		ToolCalls:       []snapshotToolCall{},
		Images:          []any{},
		Todos:           []any{},
		ThinkingHistory: []any{},
		ReportData:      nil,
	}
	if result == nil {
		return snapshot
	}

	pendingToolCalls := make(map[string]snapshotToolCall)
	for i, ev := range result.Events {
		agentName := ev.AgentName
		if agentName == "" {
			agentName = "System"
		}
		timestamp := time.Now().Format(time.RFC3339)

		if ev.Error != "" {
			snapshot.AgentActivities = append(snapshot.AgentActivities, snapshotActivity{
				ID:        fmt.Sprintf("event_%d_error", i),
				AgentName: agentName,
				Type:      "error",
				Content:   ev.Error,
				Timestamp: timestamp,
			})
			continue
		}

		if ev.Action != nil && ev.Action.TransferToAgent != nil {
			to := ev.Action.TransferToAgent.DestAgentName
			snapshot.AgentActivities = append(snapshot.AgentActivities, snapshotActivity{
				ID:        fmt.Sprintf("event_%d_transfer", i),
				AgentName: agentName,
				Type:      "transfer",
				Content:   "Delegating to " + to,
				Timestamp: timestamp,
				Metadata:  map[string]any{"transfer_to": to},
			})
			snapshot.TotalSteps++
		}

		if ev.Output == nil || ev.Output.Message == nil {
			continue
		}
		msg := ev.Output.Message

		if strings.TrimSpace(msg.ReasoningContent) != "" {
			snapshot.AgentActivities = append(snapshot.AgentActivities, snapshotActivity{
				ID:        fmt.Sprintf("event_%d_thinking", i),
				AgentName: agentName,
				Type:      "thinking",
				Content:   msg.ReasoningContent,
				Timestamp: timestamp,
			})
		}

		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				args := parseToolArguments(tc.Function.Arguments)
				call := snapshotToolCall{
					Tool:      tc.Function.Name,
					Arguments: args,
					AgentName: agentName,
				}
				pendingToolCalls[tc.ID] = call
				snapshot.AgentActivities = append(snapshot.AgentActivities, snapshotActivity{
					ID:        fmt.Sprintf("event_%d_tool_call_%s", i, tc.ID),
					AgentName: agentName,
					Type:      "tool_call",
					Content:   tc.Function.Name,
					Timestamp: timestamp,
					Metadata:  map[string]any{"arguments": args},
				})
			}
			continue
		}

		if msg.ToolCallID != "" && msg.ToolName != "" {
			call := pendingToolCalls[msg.ToolCallID]
			if call.Tool == "" {
				call.Tool = msg.ToolName
				call.Arguments = map[string]any{}
				call.AgentName = agentName
			}
			call.Success = true
			call.Output = msg.Content
			snapshot.ToolCalls = append(snapshot.ToolCalls, call)
			snapshot.TotalToolCalls++
			snapshot.AgentActivities = append(snapshot.AgentActivities, snapshotActivity{
				ID:        fmt.Sprintf("event_%d_tool_result", i),
				AgentName: agentName,
				Type:      "tool_result",
				Content:   fmt.Sprintf("%s: success", msg.ToolName),
				Timestamp: timestamp,
				Metadata:  map[string]any{"output": truncate(msg.Content, 200)},
			})
			if msg.ToolName == "emit_chart" || msg.ToolName == "read_image" {
				if image := parseImageToolOutput(msg.ToolName, msg.Content); image != nil {
					snapshot.Images = append(snapshot.Images, image)
				}
			}
			continue
		}

		if strings.TrimSpace(msg.Content) != "" {
			snapshot.AgentActivities = append(snapshot.AgentActivities, snapshotActivity{
				ID:        fmt.Sprintf("event_%d_output", i),
				AgentName: agentName,
				Type:      "output",
				Content:   msg.Content,
				Timestamp: timestamp,
			})
		}
	}
	return snapshot
}

func parseToolArguments(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err == nil && args != nil {
		return args
	}
	return map[string]any{"raw": raw}
}

func parseImageToolOutput(toolName, raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	if m, ok := parsed.(map[string]any); ok {
		if toolName == "read_image" {
			if _, hasImage := m["image_base64"]; !hasImage {
				return nil
			}
			if _, hasMime := m["mime_type"]; !hasMime {
				m["mime_type"] = "image/png"
			}
			if _, hasType := m["chart_type"]; !hasType {
				m["chart_type"] = "image"
			}
			if _, hasTitle := m["title"]; !hasTitle {
				if path, ok := m["path"].(string); ok && path != "" {
					parts := strings.Split(path, "/")
					m["title"] = parts[len(parts)-1]
				} else {
					m["title"] = "Image"
				}
			}
			return m
		}
		if _, hasType := m["chart_type"]; hasType {
			if _, hasImage := m["image_base64"]; !hasImage {
				m["image_base64"] = ""
			}
			if _, hasMime := m["mime_type"]; !hasMime {
				m["mime_type"] = "image/png"
			}
			return m
		}
	}
	return nil
}

func runSubmittedTask(h *hub.Hub, s store.SessionStore, req ChatRequest, runID, sessionID string, u *user.Info, history []hub.ConversationMessage) {
	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	ctx = user.WithContext(ctx, u)
	if req.AutoApprove != nil {
		ctx = hub.WithAutoApprove(ctx, *req.AutoApprove)
	}
	defer cancel()

	timer := time.AfterFunc(10*time.Minute, func() {
		log.Printf("[submit-task] run_id=%s session=%s idle timeout (10min) after %s, cancelling", runID, sessionID, time.Since(start).Round(time.Second))
		cancel()
	})
	defer timer.Stop()

	iter := h.RunIterWithSessionOptions(ctx, sessionID, runID, req.Message, u.ID, history, req.Model, req.LimitProfile)
	result, err := collectBackgroundEvents(iter, runID, timer)
	if err != nil {
		log.Printf("[submit-task] run_id=%s session=%s error: %v", runID, sessionID, err)
		hub.NotifyError("submit-task:"+runID, hub.ErrorParams{
			Component: "SubmitTask",
			SessionID: sessionID,
			RunID:     runID,
			UserID:    u.ID,
			Input:     truncate(req.Message, 200),
			Error:     err.Error(),
		})
		markSessionTaskStatus(s, sessionID, "failed", err.Error())
		return
	}
	result.SessionID = sessionID
	saveSessionResult(s, sessionID, result)
	markSessionTaskStatus(s, sessionID, "completed", "")
	log.Printf("[submit-task] run_id=%s session=%s done events=%d output_len=%d interrupts=%d duration=%s",
		runID, sessionID, len(result.Events), len(result.Output), len(result.Interrupts), time.Since(start).Round(time.Second))
}

func markSessionTaskStatus(s store.SessionStore, sessionID, status, errText string) {
	sess, err := s.GetSession(context.Background(), sessionID)
	if err != nil {
		return
	}
	var meta map[string]any
	if sess.Metadata != "" {
		_ = json.Unmarshal([]byte(sess.Metadata), &meta)
	}
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["task_status"] = status
	meta["task_updated_at"] = time.Now().Format(time.RFC3339)
	if errText != "" {
		meta["task_error"] = errText
	} else {
		delete(meta, "task_error")
	}
	if updated, err := json.Marshal(meta); err == nil {
		sess.Metadata = string(updated)
		_ = s.UpdateSession(context.Background(), sess)
	}
}

// stripKnsightTags removes internal control comments from persisted messages.
func stripKnsightTags(s string) string {
	cleaned := knsightTagsRe.ReplaceAllString(s, "")
	cleaned = knsightTodosRe.ReplaceAllString(cleaned, "")
	return strings.TrimRight(cleaned, " \t\r\n")
}

// 标签注释块的正则。supervisor 在最终回复末尾会追加：
//
//	<!-- knsight-tags
//	{"category": "...", "severity": "...", "tags": [...], "summary": "..."}
//	-->
//
// (?s) 让 . 匹配换行；非贪婪匹配到最近的 -->
var knsightTagsRe = regexp.MustCompile(`(?s)<!--\s*knsight-tags\s*(\{.*?\})\s*-->`)

// knsightTodosRe 匹配 supervisor 在回复中嵌入的 Plan-and-Execute 进度块：
//
//	<!-- knsight-todos [{"id":1,"content":"...","status":"in_progress"}, ...] -->
//
// 可多次出现（初始计划 + 每步完成时更新），后端扫描后发 todo_update SSE 事件。
var knsightTodosRe = regexp.MustCompile(`(?s)<!--\s*knsight-todos\s*(\[.*?\])\s*-->`)

// severity → 看板告警面板的颜色映射（沿用 socsci 的 conclusion_confidence 取值，
// 这样前端的红/黄/绿告警 pill 不需要改）：
//
//	CRITICAL → LOW（红，立即处理）
//	WARNING  → MEDIUM（黄）
//	INFO/NORMAL → HIGH（绿）
var severityToConfidence = map[string]string{
	"CRITICAL": "LOW",
	"WARNING":  "MEDIUM",
	"INFO":     "HIGH",
	"NORMAL":   "HIGH",
}

// enrichSessionTags 从 supervisor 输出末尾提取 <!-- knsight-tags ... --> 注释块，
// 解析其中的 JSON 并合并进 session.metadata。允许部分字段缺失。
func enrichSessionTags(sess *store.Session, output string) {
	matches := knsightTagsRe.FindStringSubmatch(output)
	if len(matches) < 2 {
		return
	}
	var t struct {
		Category string   `json:"category"`
		Severity string   `json:"severity"`
		Tags     []string `json:"tags"`
		Summary  string   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(matches[1]), &t); err != nil {
		log.Printf("[session %s] knsight-tags parse error: %v", sess.ID, err)
		return
	}

	var meta map[string]any
	if sess.Metadata != "" {
		_ = json.Unmarshal([]byte(sess.Metadata), &meta)
	}
	if meta == nil {
		meta = make(map[string]any)
	}

	if t.Category != "" {
		// 复用 interference_type 字段，看板的 type_distribution 直接生效。
		meta["interference_type"] = strings.ToUpper(t.Category)
	}
	if t.Severity != "" {
		sev := strings.ToUpper(t.Severity)
		meta["severity"] = sev
		if conf, ok := severityToConfidence[sev]; ok {
			meta["conclusion_confidence"] = conf
		}
	}
	if t.Summary != "" {
		meta["reasoning_summary"] = t.Summary
	}
	if len(t.Tags) > 0 {
		// 去重 + trim
		seen := make(map[string]struct{}, len(t.Tags))
		clean := make([]string, 0, len(t.Tags))
		for _, tg := range t.Tags {
			tg = strings.TrimSpace(tg)
			if tg == "" {
				continue
			}
			if _, dup := seen[tg]; dup {
				continue
			}
			seen[tg] = struct{}{}
			clean = append(clean, tg)
		}
		if len(clean) > 0 {
			meta["tags"] = clean
		}
	}

	if updated, err := json.Marshal(meta); err == nil {
		sess.Metadata = string(updated)
	}
}

// enrichSessionStatus 把"会话已完成"标记进 metadata，用于看板的 DiagnosedCount /
// confidence_dist 统计。普通版 InsightSupervisor 输出的是 markdown 报告，
// enrichSessionMetadata 那条 JSON 解析路径根本走不进去，所以 conclusion_stage 永远
// 是空，看板就误以为所有 session 都是 PENDING。
//
// 规则：
//   - 不覆盖已有值（socsci 场景里 supervisor 输出 JSON 时 enrichSessionMetadata 已经
//     写入了真实的 conclusion_stage / conclusion_confidence，那个更精准）。
//   - 只有 result.Output 非空（supervisor 真的产出过最终回复）才认为完成。
//   - 中断 / 报错的 run 不写完成标记，仍然按 PENDING 处理。
func enrichSessionStatus(sess *store.Session, result *hub.RunResult) {
	if result == nil || result.Output == "" {
		return
	}
	if len(result.Interrupts) > 0 || result.TransferError {
		return
	}

	var m map[string]any
	if sess.Metadata != "" {
		if err := json.Unmarshal([]byte(sess.Metadata), &m); err != nil {
			m = make(map[string]any)
		}
	} else {
		m = make(map[string]any)
	}
	if v, ok := m["conclusion_stage"]; !ok || v == nil || v == "" {
		m["conclusion_stage"] = "completed"
	}
	if v, ok := m["conclusion_confidence"]; !ok || v == nil || v == "" {
		m["conclusion_confidence"] = "HIGH"
	}
	if updated, err := json.Marshal(m); err == nil {
		sess.Metadata = string(updated)
	}
}

// enrichSessionTokens sums prompt+completion tokens across all events that carry
// ResponseMeta.Usage and merges the running total into session metadata as
// "token_usage" (int). Existing token_usage is replaced (the new total is
// authoritative for this Run; for incremental runs we'd need delta tracking
// which we don't have today).
func enrichSessionTokens(sess *store.Session, result *hub.RunResult) {
	if result == nil {
		return
	}
	var total int64
	for _, ev := range result.Events {
		if ev.Output == nil || ev.Output.Message == nil {
			continue
		}
		meta := ev.Output.Message.ResponseMeta
		if meta == nil || meta.Usage == nil {
			continue
		}
		total += int64(meta.Usage.PromptTokens + meta.Usage.CompletionTokens)
	}
	if total <= 0 {
		return
	}

	var m map[string]any
	if sess.Metadata != "" {
		if err := json.Unmarshal([]byte(sess.Metadata), &m); err != nil {
			m = make(map[string]any)
		}
	} else {
		m = make(map[string]any)
	}
	// 累加：这次 Run 的 total 加到已存的 token_usage 上，跨多轮对话也能正确累积。
	prev := int64(0)
	if v, ok := m["token_usage"]; ok {
		switch x := v.(type) {
		case float64:
			prev = int64(x)
		case int64:
			prev = x
		case int:
			prev = int64(x)
		}
	}
	m["token_usage"] = prev + total
	if updated, err := json.Marshal(m); err == nil {
		sess.Metadata = string(updated)
	}
}

// enrichSessionMetadata tries to parse the Supervisor output as diagnostic JSON
// and merge fields (interference_type, conclusion_confidence, conclusion_stage, etc.)
// into the session's Metadata JSON blob.
func enrichSessionMetadata(sess *store.Session, output string) {
	// Try to extract JSON from the output (Supervisor may wrap it in markdown code blocks)
	jsonStr := output
	if idx := strings.Index(output, "{"); idx >= 0 {
		if end := strings.LastIndex(output, "}"); end > idx {
			jsonStr = output[idx : end+1]
		}
	}

	var diag map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &diag); err != nil {
		return // not valid JSON, skip enrichment
	}

	// Parse existing metadata
	var meta map[string]interface{}
	if sess.Metadata != "" {
		if err := json.Unmarshal([]byte(sess.Metadata), &meta); err != nil {
			meta = make(map[string]interface{})
		}
	} else {
		meta = make(map[string]interface{})
	}

	// Copy diagnostic fields into metadata
	for _, key := range []string{
		"conclusion_stage", "interference_type", "conclusion_confidence",
		"reasoning_summary", "pipeline_trace", "top_suspects", "recommendations",
	} {
		if v, ok := diag[key]; ok {
			meta[key] = v
		}
	}

	// Marshal back to session.Metadata
	if updated, err := json.Marshal(meta); err == nil {
		sess.Metadata = string(updated)
	}
}

// isTransferToAgentError checks if an error is the known ADK resume limitation.
func isTransferToAgentError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "transfer_to_agent not found")
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// truncateRunes 按 Unicode rune 而不是 byte 截断字符串，避免在多字节 UTF-8
// 字符（如中文）中间切断产生 0xFF 替换字符 / 浏览器渲染成 "��"。
// `\n` 替换为空格，让 title 保持单行。
func truncateRunes(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(rs[:maxRunes])
	}
	return string(rs[:maxRunes-3]) + "..."
}

// logEventDetail logs every event with full content for debugging.
func logEventDetail(runID string, idx int, ev hub.PublicEvent, msg *schema.Message) {
	agent := ev.AgentName
	if agent == "" {
		agent = "?"
	}
	prefix := fmt.Sprintf("[event] run_id=%s #%d agent=%s", runID, idx, agent)

	// Interrupt check — before message processing
	if ev.Action != nil && ev.Action.Interrupted != nil {
		ctxs := ev.Action.Interrupted.InterruptContexts
		log.Printf("%s INTERRUPTED (%d contexts)", prefix, len(ctxs))
		for i, ic := range ctxs {
			log.Printf("%s  interrupt[%d] id=%s root=%v info=%v", prefix, i, ic.ID, ic.IsRootCause, ic.Info)
		}
	}

	if msg == nil {
		if ev.Action != nil {
			log.Printf("%s (no message, action: exit=%v interrupted=%v transfer=%v)",
				prefix, ev.Action.Exit, ev.Action.Interrupted != nil,
				ev.Action.TransferToAgent != nil)
		} else {
			log.Printf("%s (no message)", prefix)
		}
		return
	}

	// Assistant text content (thinking, reasoning, or final output)
	if msg.Role == schema.Assistant && msg.Content != "" && msg.ToolCallID == "" {
		log.Printf("%s output: %s", prefix, msg.Content)
	}

	// Tool calls (assistant -> tool)
	if len(msg.ToolCalls) > 0 {
		for _, tc := range msg.ToolCalls {
			log.Printf("%s -> %s args=%s", prefix, tc.Function.Name, tc.Function.Arguments)
		}
	}

	// Tool result (tool -> assistant)
	if msg.ToolCallID != "" {
		log.Printf("%s <- %s result=%s", prefix, msg.ToolName, msg.Content)
	}
}

func streamEvents(w http.ResponseWriter, iter *adk.AsyncIterator[*adk.AgentEvent], runID string, idleTimer *time.Timer, sessionID string, contextCompactionEvents <-chan hub.ContextCompactionEvent) *hub.RunResult {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx/tengine proxy buffering
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return nil
	}
	log.Printf("[stream] run_id=%s started", runID)
	// Send session_id immediately so the frontend can show the share button
	if sessionID != "" {
		sessPayload, _ := json.Marshal(StreamEnvelope{Type: "session", SessionID: sessionID})
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(sessPayload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
	result, err := collectAndStream(iter, w, flusher, runID, idleTimer, contextCompactionEvents)
	if err != nil {
		log.Printf("[stream] run_id=%s session=%s error: %v", runID, sessionID, err)
		hub.NotifyError("stream:"+runID, hub.ErrorParams{
			Component: "Stream",
			SessionID: sessionID,
			RunID:     runID,
			Error:     err.Error(),
		})
		// Still send a final event with error info so frontend can display it
		if result == nil {
			result = &hub.RunResult{RunID: runID}
		}
		if result.Output == "" {
			result.Output = fmt.Sprintf("处理出错：%v\n请重试或换个方式提问。", err)
		}
		result.SessionID = sessionID
		result.Output = stripKnsightTags(result.Output)
		payload, _ := json.Marshal(StreamEnvelope{Type: "final", Result: result})
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
		return result
	}
	result.SessionID = sessionID
	result.Output = stripKnsightTags(result.Output)
	payload, _ := json.Marshal(StreamEnvelope{Type: "final", Result: result})
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()
	log.Printf("[stream] run_id=%s done events=%d output_len=%d interrupts=%d", runID, len(result.Events), len(result.Output), len(result.Interrupts))
	return result
}

func streamOpenAIChunks(w http.ResponseWriter, iter *adk.AsyncIterator[*adk.AgentEvent], runID string, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	log.Printf("[completions-stream] run_id=%s started", runID)

	isFirst := true
	hasInterrupts := false
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			log.Printf("[completions-stream] run_id=%s error: %v", runID, event.Err)
			return
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			hasInterrupts = true
		}
		publicEvent := hub.ToPublicEvent(event)
		chunk := hub.EventToOpenAIChunk(publicEvent, runID, model, isFirst)
		if chunk != nil {
			_, _ = w.Write(hub.MarshalSSEData(chunk))
			flusher.Flush()
			isFirst = false
		}
	}

	// Send final chunk with finish_reason
	stopChunk := hub.OpenAIStopChunk(runID, model, hasInterrupts)
	_, _ = w.Write(hub.MarshalSSEData(stopChunk))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
	log.Printf("[completions-stream] run_id=%s done", runID)
}

func collectFromIter(iter *adk.AsyncIterator[*adk.AgentEvent], runID string) (*hub.RunResult, error) {
	var (
		output     string
		events     []hub.PublicEvent
		interrupts []*adk.InterruptCtx
	)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			if isTransferToAgentError(event.Err) {
				log.Printf("[iter] run_id=%s transfer_to_agent error (ADK resume limitation), flagging for retry", runID)
				return &hub.RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts, TransferError: true}, nil
			}
			// Log error but continue collecting — don't terminate the whole diagnosis
			log.Printf("[iter] run_id=%s event error (continuing): %v", runID, event.Err)
			hub.NotifyError("iter:"+runID, hub.ErrorParams{
				Component: "AgentIter",
				RunID:     runID,
				Error:     event.Err.Error(),
			})
			// Append error info to output so user/supervisor can see what failed
			errNote := fmt.Sprintf("\n\n⚠️ 中间步骤出错: %v", event.Err)
			output += errNote
			continue
		}
		publicEvent := hub.ToPublicEvent(event)
		events = append(events, publicEvent)
		if event.Action != nil && event.Action.Interrupted != nil {
			interrupts = event.Action.Interrupted.InterruptContexts
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			log.Printf("[iter] run_id=%s GetMessage error (continuing): %v", runID, err)
			continue
		}
		if shouldUseAsRunOutput(msg) {
			output = msg.Content
		}
	}
	return &hub.RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts}, nil
}

func collectBackgroundEvents(iter *adk.AsyncIterator[*adk.AgentEvent], runID string, idleTimer *time.Timer) (*hub.RunResult, error) {
	var (
		output     string
		events     []hub.PublicEvent
		interrupts []*adk.InterruptCtx
		eventCount int
	)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			if isTransferToAgentError(event.Err) {
				log.Printf("[submit-task] run_id=%s transfer_to_agent error (ADK resume limitation), flagging for retry", runID)
				return &hub.RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts, TransferError: true}, nil
			}
			log.Printf("[submit-task] run_id=%s event error (continuing): %v", runID, event.Err)
			hub.NotifyError("submit-task-event:"+runID, hub.ErrorParams{
				Component: "SubmitTaskEvent",
				RunID:     runID,
				Error:     event.Err.Error(),
			})
			output += fmt.Sprintf("\n\n⚠️ 中间步骤出错: %v", event.Err)
			continue
		}
		if idleTimer != nil {
			idleTimer.Reset(10 * time.Minute)
		}
		publicEvent := hub.ToPublicEvent(event)
		events = append(events, publicEvent)
		eventCount++
		if event.Action != nil && event.Action.Interrupted != nil {
			interrupts = event.Action.Interrupted.InterruptContexts
			log.Printf("[submit-task] run_id=%s INTERRUPT detected: %d contexts", runID, len(interrupts))
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			log.Printf("[submit-task] run_id=%s GetMessage error (continuing): %v", runID, err)
			continue
		}
		if shouldUseAsRunOutput(msg) {
			output = msg.Content
		}
		logEventDetail(runID, eventCount, publicEvent, msg)
	}
	return &hub.RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts}, nil
}

func collectAndStream(iter *adk.AsyncIterator[*adk.AgentEvent], w http.ResponseWriter, flusher http.Flusher, runID string, idleTimer *time.Timer, contextCompactionEvents <-chan hub.ContextCompactionEvent) (*hub.RunResult, error) {
	var (
		output        string
		events        []hub.PublicEvent
		interrupts    []*adk.InterruptCtx
		eventCount    int
		lastTodosJSON string // tracks last emitted todos to avoid duplicate events
	)
	type iteratorResult struct {
		event *adk.AgentEvent
		ok    bool
	}
	iteratorResults := make(chan iteratorResult, 1)
	go func() {
		for {
			event, ok := iter.Next()
			iteratorResults <- iteratorResult{event: event, ok: ok}
			if !ok {
				return
			}
		}
	}()

	for {
		var event *adk.AgentEvent
		select {
		case progress := <-contextCompactionEvents:
			_, _ = w.Write(hub.MarshalSSEData(map[string]any{
				"type":  "context_compaction",
				"event": progress,
			}))
			flusher.Flush()
			if idleTimer != nil {
				idleTimer.Reset(10 * time.Minute)
			}
			continue
		case next := <-iteratorResults:
			if !next.ok {
				return &hub.RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts}, nil
			}
			event = next.event
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			if isTransferToAgentError(event.Err) {
				log.Printf("[stream] run_id=%s transfer_to_agent error (ADK resume limitation), flagging for retry", runID)
				return &hub.RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts, TransferError: true}, nil
			}
			// Log error but continue streaming — don't terminate
			log.Printf("[stream] run_id=%s event error (continuing): %v", runID, event.Err)
			hub.NotifyError("stream-event:"+runID, hub.ErrorParams{
				Component: "StreamEvent",
				RunID:     runID,
				Error:     event.Err.Error(),
			})
			errPayload, _ := json.Marshal(map[string]string{"type": "error", "error": event.Err.Error()})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(errPayload)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
			output += fmt.Sprintf("\n\n⚠️ 中间步骤出错: %v", event.Err)
			continue
		}
		// Reset idle timer on each event — agent is still working
		if idleTimer != nil {
			idleTimer.Reset(10 * time.Minute)
		}
		publicEvent := hub.ToPublicEvent(event)
		sanitizePublicEventContent(&publicEvent)
		events = append(events, publicEvent)
		eventCount++
		if event.Action != nil && event.Action.Interrupted != nil {
			interrupts = event.Action.Interrupted.InterruptContexts
			log.Printf("[stream] run_id=%s INTERRUPT detected: %d contexts", runID, len(interrupts))
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			log.Printf("[stream] run_id=%s GetMessage error (continuing): %v", runID, err)
			continue
		}
		// Todo progress is metadata carried in assistant content. It may share
		// a message with a tool call, so extract it independently from whether
		// the message is eligible to become the run's final output.
		if todosJSON := knsightTodosJSON(msg); todosJSON != "" && todosJSON != lastTodosJSON {
			lastTodosJSON = todosJSON
			var todos any
			if jsonErr := json.Unmarshal([]byte(todosJSON), &todos); jsonErr == nil {
				_, _ = w.Write(hub.MarshalSSEData(map[string]any{
					"type":  "todo_update",
					"todos": todos,
				}))
				flusher.Flush()
			}
		}
		if shouldUseAsRunOutput(msg) {
			output = msg.Content
		}

		// Log every event with full input/output details
		logEventDetail(runID, eventCount, publicEvent, msg)

		payload, _ := json.Marshal(StreamEnvelope{Type: "event", Event: &publicEvent})
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
}

func knsightTodosJSON(msg *schema.Message) string {
	if msg == nil || msg.Role != schema.Assistant || strings.TrimSpace(msg.Content) == "" {
		return ""
	}
	matches := knsightTodosRe.FindAllStringSubmatch(msg.Content, -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return ""
	}
	return last[1]
}

func sanitizePublicEventContent(event *hub.PublicEvent) {
	if event == nil || event.Output == nil || event.Output.Message == nil {
		return
	}
	cleaned := strings.TrimLeft(stripKnsightTags(event.Output.Message.Content), "\r\n")
	if cleaned == event.Output.Message.Content {
		return
	}
	msg := *event.Output.Message
	msg.Content = cleaned
	event.Output.Message = &msg
}

func shouldUseAsRunOutput(msg *schema.Message) bool {
	return msg != nil &&
		msg.Role == schema.Assistant &&
		msg.ToolCallID == "" &&
		len(msg.ToolCalls) == 0 &&
		strings.TrimSpace(msg.Content) != "" &&
		!hub.IsStageLimitStatus(msg.Content)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
