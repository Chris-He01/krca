package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client"

	"knsight-go/internal/hub/memory"
	"knsight-go/internal/hub/sandbox"
	"knsight-go/internal/hub/skills"
	"knsight-go/internal/protocol"
	"knsight-go/internal/registry"
)

// ConfigWatcher provides fast change detection and full config reading.
// Implemented by ConfigAPI.
type ConfigWatcher interface {
	// Version returns an atomic counter incremented on every write (~1 ns to read).
	Version() int64
	// ReadFullConfig returns the current effective config (YAML + DB merged).
	ReadFullConfig() (Config, error)
}

type Hub struct {
	cfg                Config
	defaultAutoApprove bool
	runner             *adk.Runner
	store              *MemoryCheckPointStore
	mcpClients         []*client.Client
	regStore           *registry.Store
	sandbox            *sandbox.Sandbox
	toolRegistry       *ToolRegistry
	contextBuilder     *ContextBuilder
	memoryStore        *memory.MemoryStore
	sessionMgr         *memory.SessionManager
	skillsLoader       *skills.Loader
	mcpCleanups        []func()    // per-run cleanup callbacks (e.g., chunk store reset)
	toolAgents         []adk.Agent // built once at startup; reused by ephemeral runners
	llmRecorder        *llmRoundRecorder

	// hot-reload: version-gated, zero-cost on the happy path
	watcher     ConfigWatcher
	lastVersion atomic.Int64 // last seen watcher version
	reloadMu    sync.Mutex   // one reload at a time
	runnerMu    sync.RWMutex // R: Run/RunIter hold snapshot; W: rebuildRunner swaps
	lastCfg     Config       // last applied config (protected by reloadMu)
}

type RunResult struct {
	RunID         string              `json:"run_id"`
	SessionID     string              `json:"session_id,omitempty"`
	Output        string              `json:"output,omitempty"`
	Events        []PublicEvent       `json:"events,omitempty"`
	Interrupts    []*adk.InterruptCtx `json:"interrupts,omitempty"`
	TransferError bool                `json:"transfer_error,omitempty"` // true if transfer_to_agent failed (ADK resume bug)
}

func NewHub(ctx context.Context, cfg Config, opts ...HubOption) (*Hub, error) {
	cfg.ApplyDefaults()

	o := &hubOptions{}
	for _, opt := range opts {
		opt(o)
	}

	baseModel, err := NewFailoverChatModel(ctx, cfg.LLM)
	if err != nil {
		return nil, err
	}

	// Build tool agents (MCP + external) — these are the bottom layer
	recorder := newLLMRoundRecorder(o.llmRoundSink)
	toolAgents, mcpClients, mcpCleanups, err := buildToolAgents(ctx, baseModel, cfg, o.regStore, recorder)
	if err != nil {
		return nil, err
	}

	// Initialize optional subsystems
	var memStore *memory.MemoryStore
	var sessionMgr *memory.SessionManager
	if cfg.Memory.Enabled && cfg.Memory.WorkspaceDir != "" {
		memStore = memory.NewMemoryStore(cfg.Memory.WorkspaceDir, cfg.Memory.RecentDays)
		sessionMgr = memory.NewSessionManager(cfg.Memory.WorkspaceDir, cfg.Memory.MaxMessages)
		log.Printf("memory system enabled (workspace=%s)", cfg.Memory.WorkspaceDir)
	}

	var skillsLoader *skills.Loader
	if cfg.Skills.Enabled {
		skillsLoader = skills.NewMultiLoader(cfg.Skills.DefaultSkillDir, cfg.Skills.SkillDir)
		if err := skillsLoader.Load(); err != nil {
			log.Printf("warning: skills load error: %v", err)
		} else {
			log.Printf("skills loaded: %d skills (default=%s, user=%s)",
				len(skillsLoader.ListSkills()), cfg.Skills.DefaultSkillDir, cfg.Skills.SkillDir)
		}
	}

	ctxBuilder := NewContextBuilder(memStore, skillsLoader, cfg.Skills.DataDir, cfg.SceneID)

	// Build sandbox tools
	var sb *sandbox.Sandbox
	toolReg := NewToolRegistry()

	if cfg.Sandbox.Enabled {
		sb, err = sandbox.New(sandbox.Config{
			WorkspaceDir:        cfg.Sandbox.WorkspaceDir,
			DataDir:             cfg.Skills.DataDir,
			DenyPatterns:        cfg.Sandbox.DenyPatterns,
			MaxOutputBytes:      cfg.Sandbox.MaxOutputBytes,
			CommandTimeoutSec:   cfg.Sandbox.CommandTimeoutSec,
			RestrictToWorkspace: cfg.Sandbox.RestrictToWorkspace,
			SessionBaseDir:      cfg.Sandbox.SessionBaseDir,
			SessionMaxAgeSec:    cfg.Sandbox.SessionMaxAgeSec,
		})
		if err != nil {
			return nil, fmt.Errorf("init sandbox: %w", err)
		}
		toolReg.Register(sandbox.NewExecShellTool(sb))
		toolReg.Register(sandbox.NewReadFileTool(sb))
		toolReg.Register(sandbox.NewWriteFileTool(sb))
		toolReg.Register(sandbox.NewListDirTool(sb))
		toolReg.Register(sandbox.NewReadImageTool(sb))
		toolReg.Register(sandbox.NewEmitChartTool(sb))

		if cfg.Sandbox.WebFetchEnabled {
			toolReg.Register(sandbox.NewWebFetchTool(cfg.Sandbox.WebFetchTimeoutSec, cfg.Sandbox.WebFetchMaxBodyBytes))
		}
		log.Printf("sandbox enabled with %d tools", len(toolReg.GetTools()))
	}

	// Register memory tools (available even without sandbox)
	if memStore != nil {
		toolReg.Register(memory.NewReadMemoryTool(memStore))
		toolReg.Register(memory.NewUpdateMemoryTool(memStore))
		toolReg.Register(memory.NewAppendJournalTool(memStore))
		log.Printf("memory tools registered (read_memory, update_memory, append_journal)")
	}

	// Register skill retrieval tool
	if skillsLoader != nil {
		toolReg.Register(skills.NewGetSkillToolWithScene(skillsLoader, cfg.SceneID))
		log.Printf("skill tool registered (get_skill, %d skills available, scene=%q)", len(skillsLoader.ListSkills()), cfg.SceneID)
	}

	// Build supervisor tools (sandbox + approval, no MCP)
	sandboxTools := toolReg.GetTools()
	sandboxTools = WrapToolsWithApproval(sandboxTools)
	supervisorTools := assembleSupervisorTools(sandboxTools)

	// Build system prompt via ContextBuilder
	supervisorInstruction := ctxBuilder.BuildSystemPrompt(cfg.Supervisor, nil)

	log.Printf("[hub] supervisor %q max_iterations=%d timeout=%ds", cfg.Supervisor.Name, cfg.Supervisor.MaxIterations, cfg.Supervisor.TimeoutSeconds)
	supervisorAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Supervisor.Name,
		Description:   cfg.Supervisor.Description,
		Instruction:   supervisorInstruction,
		Model:         withLLMRecording(baseModel, cfg.Supervisor.Name, recorder),
		ToolsConfig:   recoverableToolsConfig(supervisorTools),
		MaxIterations: cfg.Supervisor.MaxIterations,
		Middlewares:   []adk.AgentMiddleware{{BeforeChatModel: NewLoopDetector()}},
	})
	if err != nil {
		return nil, err
	}

	// Build middle-layer sub-agents, each wrapping tool agents as a sub-supervisor
	taNames := toolAgentNames(ctx, toolAgents)
	log.Printf("available tool agents for sub-supervisors: %v", taNames)
	subAgents := make([]adk.Agent, 0, len(cfg.SubAgents))
	var summaryAgent adk.Agent
	for _, agentCfg := range cfg.SubAgents {
		agentToolAgents := toolAgents
		if agentCfg.SandboxOnly {
			agentToolAgents = nil // sandbox-only agents cannot delegate to external tool agents
			log.Printf("[hub] %s: sandbox_only=true, no tool agents attached", agentCfg.Name)
		}
		agent, err := buildSubSupervisor(ctx, baseModel, agentCfg, agentToolAgents, sandboxTools, ctxBuilder, recorder)
		if err != nil {
			return nil, err
		}
		subAgents = append(subAgents, agent)
		if agentCfg.Name == "SummaryAgent" {
			summaryAgent = agent
		}
	}

	// --- Token budget estimation ---
	logTokenBudget(ctx, cfg, supervisorInstruction, supervisorTools, toolAgents, sandboxTools, ctxBuilder, memStore)

	root, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: supervisorAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		return nil, err
	}
	var rootAgent adk.Agent = root
	rootAgent = withAgentFinalizer(
		rootAgent,
		time.Duration(cfg.Supervisor.TimeoutSeconds)*time.Second,
		cfg.Supervisor.MaxIterations,
		summaryAgent,
	)

	cpStore := NewMemoryCheckPointStore()
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           rootAgent,
		EnableStreaming: false,
		CheckPointStore: cpStore,
	})

	h := &Hub{
		cfg:                cfg,
		defaultAutoApprove: cfg.Sandbox.IsAutoApprove(),
		runner:             runner,
		store:              cpStore,
		mcpClients:         mcpClients,
		regStore:           o.regStore,
		sandbox:            sb,
		toolRegistry:       toolReg,
		contextBuilder:     ctxBuilder,
		memoryStore:        memStore,
		sessionMgr:         sessionMgr,
		skillsLoader:       skillsLoader,
		mcpCleanups:        mcpCleanups,
		toolAgents:         toolAgents,
		llmRecorder:        recorder,
	}

	return h, nil
}

// HubOption configures optional Hub dependencies.
type HubOption func(*hubOptions)

type hubOptions struct {
	regStore     *registry.Store
	llmRoundSink LLMRoundSink
}

// WithRegistryStore embeds the registry store directly (no HTTP needed).
func WithRegistryStore(s *registry.Store) HubOption {
	return func(o *hubOptions) {
		o.regStore = s
	}
}

// WithLLMRoundSink records every agent-to-LLM request and response.
func WithLLMRoundSink(sink LLMRoundSink) HubOption {
	return func(o *hubOptions) {
		o.llmRoundSink = sink
	}
}

// MemoryStore returns the hub's memory store (may be nil if memory is disabled).
func (h *Hub) MemoryStore() *memory.MemoryStore {
	return h.memoryStore
}

// SceneID returns the active scene identifier (from config, set via KNSIGHT_SCENE env var).
// Returns empty string when running in default (non-scene) mode.
func (h *Hub) SceneID() string {
	return h.cfg.SceneID
}

// AvailableModels returns the user-selectable model list from the current config.
func (h *Hub) AvailableModels() []UserSelectableModel {
	return h.cfg.UserSelectableModels
}

// DefaultModelID returns the configured system-default LLM model identifier.
func (h *Hub) DefaultModelID() string {
	return h.cfg.LLM.Model
}

func (h *Hub) AvailableRunLimitProfiles() []RunLimitProfile {
	h.runnerMu.RLock()
	defer h.runnerMu.RUnlock()
	return append([]RunLimitProfile(nil), h.cfg.RunLimitProfiles...)
}

// ResolveUserModel looks up a UserSelectableModel by label or model_id.
// Returns nil when the label is empty or equals "Knsight" (use system default).
func (h *Hub) ResolveUserModel(labelOrID string) *UserSelectableModel {
	if labelOrID == "" || labelOrID == "Knsight" {
		return nil
	}
	for i := range h.cfg.UserSelectableModels {
		m := &h.cfg.UserSelectableModels[i]
		if m.Label == labelOrID || m.ModelID == labelOrID {
			if !m.IsDefault() {
				return m
			}
		}
	}
	return nil
}

// buildLLMConfigForModel derives an LLMConfig for a user-selected model by
// merging override fields on top of the base LLM config.
func (h *Hub) buildLLMConfigForModel(sel UserSelectableModel) LLMConfig {
	cfg := h.cfg.LLM
	cfg.Model = sel.ModelID
	// User-selectable models only send max_tokens when explicitly configured.
	cfg.MaxTokens = sel.MaxTokens
	if sel.BaseURL != "" {
		cfg.BaseURL = sel.BaseURL
	}
	if sel.APIKey != "" {
		cfg.APIKey = sel.APIKey
	}
	return cfg
}

// SetConfigWatcher registers a ConfigWatcher for hot-reload.
// Call once after NewHub, before serving requests.
func (h *Hub) SetConfigWatcher(w ConfigWatcher) {
	h.watcher = w
	h.lastVersion.Store(w.Version()) // treat current version as "already applied"
	h.lastCfg = h.cfg
}

// CheckAndReloadIfChanged is the single entry point for hot-reload.
// Call it at the top of each chat request handler.
//
// Fast path (no change): one atomic load (~1 ns), returns immediately.
// Slow path (change detected): reads full config, diffs sections, and reloads
// only what changed — lightweight (memory/skills) or heavy (rebuild runner).
func (h *Hub) CheckAndReloadIfChanged() {
	if h.watcher == nil {
		return
	}
	// Fast path: atomic compare — zero DB/disk I/O.
	current := h.watcher.Version()
	if current == h.lastVersion.Load() {
		return
	}

	// Slow path: something changed — ensure only one reload runs at a time.
	h.reloadMu.Lock()
	defer h.reloadMu.Unlock()

	// Re-check after acquiring lock; another goroutine may have already reloaded.
	if current == h.lastVersion.Load() {
		return
	}

	newCfg, err := h.watcher.ReadFullConfig()
	if err != nil {
		log.Printf("[hub] hot-reload: read config error: %v", err)
		return
	}

	if ok := h.applyConfigChanges(newCfg); ok {
		h.lastVersion.Store(current)
	}
}

// applyConfigChanges diffs newCfg against the last applied config and triggers
// the minimum necessary reload for each changed section.
// Returns false if a runner rebuild was attempted but failed (version not advanced).
func (h *Hub) applyConfigChanges(newCfg Config) bool {
	old := h.lastCfg

	memChanged := newCfg.Memory != old.Memory    // MemoryConfig has only basic types
	skillsChanged := newCfg.Skills != old.Skills // SkillsConfig has only basic types
	heavyChanged := !jsonEqual(newCfg.LLM, old.LLM) ||
		!jsonEqual(newCfg.Supervisor, old.Supervisor) ||
		!jsonEqual(newCfg.SubAgents, old.SubAgents) ||
		!jsonEqual(newCfg.Tools, old.Tools) ||
		!jsonEqual(newCfg.Sandbox, old.Sandbox)

	if memChanged {
		h.hotReloadMemory(newCfg.Memory)
	}
	if skillsChanged {
		h.hotReloadSkills(newCfg)
	}
	if heavyChanged {
		// Skills context also needs refreshing when the runner is rebuilt.
		if !skillsChanged {
			h.hotReloadSkills(newCfg)
		}
		if !h.rebuildRunner(newCfg) {
			return false
		}
	}

	h.lastCfg = newCfg
	log.Printf("[hub] config hot-reload applied (mem=%v skills=%v runner=%v)",
		memChanged, skillsChanged, heavyChanged)
	return true
}

// hotReloadMemory rebuilds the memory subsystem in place (no runner rebuild needed).
func (h *Hub) hotReloadMemory(cfg MemoryConfig) {
	if !cfg.Enabled || cfg.WorkspaceDir == "" {
		if h.memoryStore != nil {
			log.Printf("[hub] memory disabled — clearing store")
		}
		h.memoryStore = nil
		h.sessionMgr = nil
		h.contextBuilder.memoryStore = nil
		return
	}
	h.memoryStore = memory.NewMemoryStore(cfg.WorkspaceDir, cfg.RecentDays)
	h.sessionMgr = memory.NewSessionManager(cfg.WorkspaceDir, cfg.MaxMessages)
	h.contextBuilder.memoryStore = h.memoryStore
	log.Printf("[hub] memory hot-reloaded (workspace=%s, recent_days=%d)", cfg.WorkspaceDir, cfg.RecentDays)
}

// hotReloadSkills reloads skill files from disk (no runner rebuild needed).
func (h *Hub) hotReloadSkills(cfg Config) {
	if !cfg.Skills.Enabled || cfg.Skills.SkillDir == "" {
		h.skillsLoader = nil
		h.contextBuilder.skillsLoader = nil
		return
	}
	// Rebuild loader if the dirs changed; otherwise just reload files in place.
	if h.skillsLoader == nil ||
		h.lastCfg.Skills.SkillDir != cfg.Skills.SkillDir ||
		h.lastCfg.Skills.DefaultSkillDir != cfg.Skills.DefaultSkillDir {
		h.skillsLoader = skills.NewMultiLoader(cfg.Skills.DefaultSkillDir, cfg.Skills.SkillDir)
	}
	if err := h.skillsLoader.Load(); err != nil {
		log.Printf("[hub] skills reload error: %v", err)
	} else {
		log.Printf("[hub] skills hot-reloaded: %d skills (default=%s user=%s)",
			len(h.skillsLoader.ListSkills()), cfg.Skills.DefaultSkillDir, cfg.Skills.SkillDir)
	}
	h.contextBuilder.skillsLoader = h.skillsLoader
}

// rebuildRunner rebuilds the full agent stack (baseModel → toolAgents → subAgents →
// supervisor → runner) and atomically swaps it in. Old MCP connections are closed
// after a short grace period so in-flight requests can complete.
// Returns true on success, false if any step failed (runner is not replaced on failure).
func (h *Hub) rebuildRunner(cfg Config) bool {
	ctx := context.Background()

	baseModel, err := NewFailoverChatModel(ctx, cfg.LLM)
	if err != nil {
		log.Printf("[hub] rebuild: model error: %v", err)
		return false
	}

	toolAgents, newMCPClients, newMCPCleanups, err := buildToolAgents(ctx, baseModel, cfg, h.regStore, h.llmRecorder)
	if err != nil {
		log.Printf("[hub] rebuild: tool agents error: %v", err)
		return false
	}

	ctxBuilder := NewContextBuilder(h.memoryStore, h.skillsLoader, cfg.Skills.DataDir, cfg.SceneID)

	var sb *sandbox.Sandbox
	toolReg := NewToolRegistry()
	if cfg.Sandbox.Enabled {
		sb, err = sandbox.New(sandbox.Config{
			WorkspaceDir:        cfg.Sandbox.WorkspaceDir,
			DataDir:             cfg.Skills.DataDir,
			DenyPatterns:        cfg.Sandbox.DenyPatterns,
			MaxOutputBytes:      cfg.Sandbox.MaxOutputBytes,
			CommandTimeoutSec:   cfg.Sandbox.CommandTimeoutSec,
			RestrictToWorkspace: cfg.Sandbox.RestrictToWorkspace,
			SessionBaseDir:      cfg.Sandbox.SessionBaseDir,
			SessionMaxAgeSec:    cfg.Sandbox.SessionMaxAgeSec,
		})
		if err != nil {
			log.Printf("[hub] rebuild: sandbox error: %v", err)
			return false
		}
		toolReg.Register(sandbox.NewExecShellTool(sb))
		toolReg.Register(sandbox.NewReadFileTool(sb))
		toolReg.Register(sandbox.NewWriteFileTool(sb))
		toolReg.Register(sandbox.NewListDirTool(sb))
		toolReg.Register(sandbox.NewReadImageTool(sb))
		toolReg.Register(sandbox.NewEmitChartTool(sb))
		if cfg.Sandbox.WebFetchEnabled {
			toolReg.Register(sandbox.NewWebFetchTool(cfg.Sandbox.WebFetchTimeoutSec, cfg.Sandbox.WebFetchMaxBodyBytes))
		}
	}
	if h.memoryStore != nil {
		toolReg.Register(memory.NewReadMemoryTool(h.memoryStore))
		toolReg.Register(memory.NewUpdateMemoryTool(h.memoryStore))
		toolReg.Register(memory.NewAppendJournalTool(h.memoryStore))
	}
	if h.skillsLoader != nil {
		toolReg.Register(skills.NewGetSkillToolWithScene(h.skillsLoader, h.cfg.SceneID))
	}

	sandboxTools := toolReg.GetTools()
	sandboxTools = WrapToolsWithApproval(sandboxTools)
	supervisorTools := assembleSupervisorTools(sandboxTools)
	supervisorInstruction := ctxBuilder.BuildSystemPrompt(cfg.Supervisor, nil)

	supervisorAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Supervisor.Name,
		Description:   cfg.Supervisor.Description,
		Instruction:   supervisorInstruction,
		Model:         withLLMRecording(baseModel, cfg.Supervisor.Name, h.llmRecorder),
		ToolsConfig:   recoverableToolsConfig(supervisorTools),
		MaxIterations: cfg.Supervisor.MaxIterations,
		Middlewares:   []adk.AgentMiddleware{{BeforeChatModel: NewLoopDetector()}},
	})
	if err != nil {
		log.Printf("[hub] rebuild: supervisor error: %v", err)
		return false
	}

	subAgents := make([]adk.Agent, 0, len(cfg.SubAgents))
	var summaryAgent adk.Agent
	for _, agentCfg := range cfg.SubAgents {
		agentToolAgents := toolAgents
		if agentCfg.SandboxOnly {
			agentToolAgents = nil
		}
		agent, err := buildSubSupervisor(ctx, baseModel, agentCfg, agentToolAgents, sandboxTools, ctxBuilder, h.llmRecorder)
		if err != nil {
			log.Printf("[hub] rebuild: sub-agent %q error: %v", agentCfg.Name, err)
			return false
		}
		subAgents = append(subAgents, agent)
		if agentCfg.Name == "SummaryAgent" {
			summaryAgent = agent
		}
	}

	root, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: supervisorAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		log.Printf("[hub] rebuild: supervisor.New error: %v", err)
		return false
	}
	var rootAgent adk.Agent = root
	rootAgent = withAgentFinalizer(
		rootAgent,
		time.Duration(cfg.Supervisor.TimeoutSeconds)*time.Second,
		cfg.Supervisor.MaxIterations,
		summaryAgent,
	)

	cpStore := NewMemoryCheckPointStore()
	newRunner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           rootAgent,
		EnableStreaming: false,
		CheckPointStore: cpStore,
	})

	// Atomically swap in the new runner.
	h.runnerMu.Lock()
	oldClients := h.mcpClients
	h.runner = newRunner
	h.store = cpStore
	h.mcpClients = newMCPClients
	h.mcpCleanups = newMCPCleanups
	h.sandbox = sb
	h.toolRegistry = toolReg
	h.contextBuilder = ctxBuilder
	h.cfg = cfg
	h.toolAgents = toolAgents
	h.runnerMu.Unlock()

	// Close old MCP connections after a grace period for in-flight requests.
	go func() {
		time.Sleep(10 * time.Second)
		for _, c := range oldClients {
			_ = c.Close()
		}
		log.Printf("[hub] old MCP connections closed after grace period")
	}()

	log.Printf("[hub] agent runner rebuilt (supervisor=%s, sub_agents=%d, llm=%s)",
		cfg.Supervisor.Name, len(cfg.SubAgents), cfg.LLM.Model)
	return true
}

// jsonEqual reports whether a and b marshal to the same JSON.
// Used for diffing config sections that contain slices or pointers.
func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// SkillsLoader returns the hub's skills loader (may be nil if skills are disabled).
func (h *Hub) SkillsLoader() *skills.Loader {
	return h.skillsLoader
}

// DebugPrompts returns all assembled prompts for debugging.
func (h *Hub) DebugPrompts(userID string) map[string]string {
	result := make(map[string]string)

	// Supervisor system prompt
	result["supervisor_instruction"] = h.contextBuilder.BuildSystemPrompt(h.cfg.Supervisor, nil)

	// Sub-agent prompts
	ctx := context.Background()
	var toolAgentNameList []string
	// Collect tool agent names from config
	for _, mcpCfg := range h.cfg.Tools.MCPs {
		toolAgentNameList = append(toolAgentNameList, mcpCfg.Name)
	}
	for _, extCfg := range h.cfg.Tools.Agents {
		toolAgentNameList = append(toolAgentNameList, extCfg.Name)
	}

	for _, agentCfg := range h.cfg.SubAgents {
		result["sub_agent_"+agentCfg.Name] = h.contextBuilder.BuildSubAgentPrompt(agentCfg, toolAgentNameList)
	}

	// Memory context (what gets injected per-request)
	if h.memoryStore != nil {
		result["memory_shared"] = h.memoryStore.GetMemoryContext()
		if userID != "" && userID != "visitor" {
			result["memory_user_"+userID] = h.memoryStore.ForUser(userID).GetMemoryContext()
		}
	}

	// Config summary
	result["config_llm_model"] = h.cfg.LLM.Model
	result["config_llm_base_url"] = h.cfg.LLM.BaseURL
	result["config_tool_agents"] = strings.Join(toolAgentNameList, ", ")

	_ = ctx
	return result
}

func (h *Hub) Close() error {
	var firstErr error
	for _, c := range h.mcpClients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func NewRunID() string {
	return uuid.NewString()
}

// ConversationMessage is a simple role+content pair for history injection.
type ConversationMessage struct {
	Role    string
	Content string
}

func (h *Hub) Run(ctx context.Context, runID string, message string, userID string, history []ConversationMessage) (*RunResult, error) {
	return h.RunWithModel(ctx, runID, message, userID, history, "")
}

// RunWithModel is like Run but allows the caller to pass a user-selected model
// label or model_id. Empty string / "Knsight" uses the system default runner.
func (h *Hub) RunWithModel(ctx context.Context, runID string, message string, userID string, history []ConversationMessage, userModel string) (*RunResult, error) {
	return h.RunWithOptions(ctx, runID, message, userID, history, userModel, "")
}

func (h *Hub) RunWithOptions(ctx context.Context, runID string, message string, userID string, history []ConversationMessage, userModel, limitProfile string) (*RunResult, error) {
	return h.RunWithSessionOptions(ctx, "", runID, message, userID, history, userModel, limitProfile)
}

// RunWithSessionOptions associates LLM traces with both a session and an individual run.
func (h *Hub) RunWithSessionOptions(ctx context.Context, sessionID, runID string, message string, userID string, history []ConversationMessage, userModel, limitProfile string) (*RunResult, error) {
	ctx = withLLMTraceContext(ctx, sessionID, runID, userID)
	h.runnerMu.RLock()
	cleanups := h.mcpCleanups
	h.runnerMu.RUnlock()
	for _, fn := range cleanups {
		fn()
	}
	if _, ok := AutoApproveFromContext(ctx); !ok {
		ctx = WithAutoApprove(ctx, h.defaultAutoApprove)
	}
	h.ensureUserDirs(userID)
	if h.sandbox != nil {
		h.sandbox.CleanExpiredRunWorkspaces()
		_ = h.sandbox.RunWorkspaceDir(runID, userID)
	}
	h.toolRegistry.SetContext(runID, userID)
	message = h.injectMemoryContext(message, userID)
	if len(history) > 0 {
		histPairs := make([]struct{ Role, Content string }, len(history))
		for i, m := range history {
			histPairs[i] = struct{ Role, Content string }{m.Role, m.Content}
		}
		message = h.injectConversationHistory(message, histPairs)
	}
	runner, err := h.runnerForOptions(ctx, userModel, limitProfile)
	if err != nil {
		return nil, err
	}
	iter := runner.Query(ctx, message, adk.WithCheckPointID(runID))
	return collectResult(runID, iter)
}

func (h *Hub) Resume(ctx context.Context, runID string, targets map[string]any) (*RunResult, error) {
	return h.ResumeWithSession(ctx, "", runID, targets)
}

// ResumeWithSession associates resumed LLM rounds with their originating session.
func (h *Hub) ResumeWithSession(ctx context.Context, sessionID, runID string, targets map[string]any) (*RunResult, error) {
	ctx = withLLMTraceContext(ctx, sessionID, runID, "")
	if h.sandbox != nil {
		h.sandbox.CleanExpiredRunWorkspaces()
		_ = h.sandbox.RunWorkspaceDir(runID, "")
	}
	h.toolRegistry.SetContext(runID, "")
	h.runnerMu.RLock()
	runner := h.runner
	h.runnerMu.RUnlock()
	var (
		iter *adk.AsyncIterator[*adk.AgentEvent]
		err  error
	)
	if len(targets) > 0 {
		iter, err = runner.ResumeWithParams(ctx, runID, &adk.ResumeParams{Targets: targets})
	} else {
		iter, err = runner.Resume(ctx, runID)
	}
	if err != nil {
		return nil, err
	}
	return collectResult(runID, iter)
}

func (h *Hub) RunIter(ctx context.Context, runID string, message string, userID string, history []ConversationMessage) *adk.AsyncIterator[*adk.AgentEvent] {
	return h.RunIterWithModel(ctx, runID, message, userID, history, "")
}

// RunIterWithModel is like RunIter but allows the caller to pass a user-selected model
// label or model_id. Empty string / "Knsight" uses the system default runner.
func (h *Hub) RunIterWithModel(ctx context.Context, runID string, message string, userID string, history []ConversationMessage, userModel string) *adk.AsyncIterator[*adk.AgentEvent] {
	return h.RunIterWithOptions(ctx, runID, message, userID, history, userModel, "")
}

func (h *Hub) RunIterWithOptions(ctx context.Context, runID string, message string, userID string, history []ConversationMessage, userModel, limitProfile string) *adk.AsyncIterator[*adk.AgentEvent] {
	return h.RunIterWithSessionOptions(ctx, "", runID, message, userID, history, userModel, limitProfile)
}

// RunIterWithSessionOptions is the streaming form of RunWithSessionOptions.
func (h *Hub) RunIterWithSessionOptions(ctx context.Context, sessionID, runID string, message string, userID string, history []ConversationMessage, userModel, limitProfile string) *adk.AsyncIterator[*adk.AgentEvent] {
	ctx = withLLMTraceContext(ctx, sessionID, runID, userID)
	if _, ok := AutoApproveFromContext(ctx); !ok {
		ctx = WithAutoApprove(ctx, h.defaultAutoApprove)
	}
	h.ensureUserDirs(userID)
	if h.sandbox != nil {
		h.sandbox.CleanExpiredRunWorkspaces()
		_ = h.sandbox.RunWorkspaceDir(runID, userID)
	}
	h.toolRegistry.SetContext(runID, userID)
	message = h.injectMemoryContext(message, userID)
	if len(history) > 0 {
		histPairs := make([]struct{ Role, Content string }, len(history))
		for i, m := range history {
			histPairs[i] = struct{ Role, Content string }{m.Role, m.Content}
		}
		message = h.injectConversationHistory(message, histPairs)
	}
	runner, err := h.runnerForOptions(ctx, userModel, limitProfile)
	if err != nil {
		log.Printf("[hub] RunIterWithOptions: build runner for model=%q limits=%q: %v — falling back to default", userModel, limitProfile, err)
		h.runnerMu.RLock()
		runner = h.runner
		h.runnerMu.RUnlock()
	}
	return runner.Query(ctx, message, adk.WithCheckPointID(runID))
}

// runnerForOptions returns the default runner for the default model and limits.
// Other selections use a request-scoped runner that reuses connected tool agents.
func (h *Hub) runnerForOptions(ctx context.Context, userModel, limitProfile string) (*adk.Runner, error) {
	h.runnerMu.RLock()
	defaultRunner := h.runner
	cfg := h.cfg
	h.runnerMu.RUnlock()

	sel := h.ResolveUserModel(userModel)
	effectiveCfg, profileApplied, err := applyRunLimitProfile(cfg, limitProfile)
	if err != nil {
		return nil, err
	}
	if sel == nil && !profileApplied {
		return defaultRunner, nil
	}

	llmCfg := cfg.LLM
	if sel != nil {
		llmCfg = h.buildLLMConfigForModel(*sel)
		log.Printf("[hub] user-selected model %q -> base_url=%s model=%s", userModel, llmCfg.BaseURL, llmCfg.Model)
	}

	baseModel, err := NewFailoverChatModel(ctx, llmCfg)
	if err != nil {
		return nil, fmt.Errorf("build model for %q: %w", userModel, err)
	}
	return h.buildEphemeralRunner(ctx, baseModel, effectiveCfg)
}

func applyRunLimitProfile(cfg Config, profileID string) (Config, bool, error) {
	if profileID == "" || profileID == "standard" {
		return cfg, false, nil
	}
	for _, profile := range cfg.RunLimitProfiles {
		if profile.ID != profileID {
			continue
		}
		if profile.PreserveConfigured {
			return cfg, false, nil
		}
		cfg.SubAgents = append([]AgentConfig(nil), cfg.SubAgents...)
		apply := func(agent *AgentConfig) {
			if profile.MaxIterations > agent.MaxIterations {
				agent.MaxIterations = profile.MaxIterations
			}
			if profile.TimeoutSeconds > agent.TimeoutSeconds {
				agent.TimeoutSeconds = profile.TimeoutSeconds
			}
		}
		apply(&cfg.Supervisor)
		for i := range cfg.SubAgents {
			apply(&cfg.SubAgents[i])
		}
		log.Printf("[hub] run limit profile %q applied: min_iterations=%d min_timeout=%ds",
			profile.ID, profile.MaxIterations, profile.TimeoutSeconds)
		return cfg, true, nil
	}
	return Config{}, false, fmt.Errorf("unknown run limit profile %q", profileID)
}

// buildEphemeralRunner constructs a runner that uses the provided baseModel
// while reusing the already-connected toolAgents from the hub (no MCP reconnection).
func (h *Hub) buildEphemeralRunner(ctx context.Context, baseModel *FailoverChatModel, cfg Config) (*adk.Runner, error) {
	h.runnerMu.RLock()
	toolAgents := h.toolAgents
	toolReg := h.toolRegistry
	ctxBuilder := h.contextBuilder
	h.runnerMu.RUnlock()

	sandboxTools := toolReg.GetTools()
	sandboxTools = WrapToolsWithApproval(sandboxTools)
	supervisorTools := assembleSupervisorTools(sandboxTools)
	supervisorInstruction := ctxBuilder.BuildSystemPrompt(cfg.Supervisor, nil)

	supervisorAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Supervisor.Name,
		Description:   cfg.Supervisor.Description,
		Instruction:   supervisorInstruction,
		Model:         withLLMRecording(baseModel, cfg.Supervisor.Name, h.llmRecorder),
		ToolsConfig:   recoverableToolsConfig(supervisorTools),
		MaxIterations: cfg.Supervisor.MaxIterations,
		Middlewares:   []adk.AgentMiddleware{{BeforeChatModel: NewLoopDetector()}},
	})
	if err != nil {
		return nil, fmt.Errorf("build supervisor: %w", err)
	}

	subAgents := make([]adk.Agent, 0, len(cfg.SubAgents))
	var summaryAgent adk.Agent
	for _, agentCfg := range cfg.SubAgents {
		agentToolAgents := toolAgents
		if agentCfg.SandboxOnly {
			agentToolAgents = nil
		}
		agent, err := buildSubSupervisor(ctx, baseModel, agentCfg, agentToolAgents, sandboxTools, ctxBuilder, h.llmRecorder)
		if err != nil {
			return nil, fmt.Errorf("build sub-agent %q: %w", agentCfg.Name, err)
		}
		subAgents = append(subAgents, agent)
		if agentCfg.Name == "SummaryAgent" {
			summaryAgent = agent
		}
	}

	root, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: supervisorAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		return nil, fmt.Errorf("supervisor.New: %w", err)
	}
	var rootAgent adk.Agent = root
	rootAgent = withAgentFinalizer(
		rootAgent,
		time.Duration(cfg.Supervisor.TimeoutSeconds)*time.Second,
		cfg.Supervisor.MaxIterations,
		summaryAgent,
	)

	cpStore := NewMemoryCheckPointStore()
	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           rootAgent,
		EnableStreaming: false,
		CheckPointStore: cpStore,
	}), nil
}

// ensureUserDirs creates per-user (or per-scene) subdirectories for sandbox, memory, and skills on first access.
func (h *Hub) ensureUserDirs(userID string) {
	h.runnerMu.RLock()
	cfg := h.cfg
	h.runnerMu.RUnlock()

	sceneID := cfg.SceneID

	// Scene mode: ensure scene-scoped directories
	if sceneID != "" {
		if cfg.Sandbox.Enabled && cfg.Sandbox.WorkspaceDir != "" {
			_ = os.MkdirAll(filepath.Join(cfg.Sandbox.WorkspaceDir, "scene", sceneID), 0755)
		}
		if cfg.Memory.Enabled && cfg.Memory.WorkspaceDir != "" {
			_ = os.MkdirAll(filepath.Join(cfg.Memory.WorkspaceDir, "memory", "scene", sceneID), 0755)
		}
		return
	}

	// Default mode: ensure user-scoped directories
	if userID == "" || userID == "visitor" {
		return
	}
	if cfg.Sandbox.Enabled && cfg.Sandbox.WorkspaceDir != "" {
		_ = os.MkdirAll(filepath.Join(cfg.Sandbox.WorkspaceDir, "user", userID), 0755)
	}
	if cfg.Memory.Enabled && cfg.Memory.WorkspaceDir != "" {
		_ = os.MkdirAll(filepath.Join(cfg.Memory.WorkspaceDir, "memory", "user", userID), 0755)
	}
	if cfg.Skills.Enabled && cfg.Skills.SkillDir != "" {
		_ = os.MkdirAll(filepath.Join(cfg.Skills.SkillDir, "user", userID), 0755)
	}
}

// injectConversationHistory prepends previous chat messages to the current message
// so the LLM has context for follow-up questions within the same session.
func (h *Hub) injectConversationHistory(message string, history []struct{ Role, Content string }) string {
	if len(history) == 0 {
		return message
	}
	var sb strings.Builder
	sb.WriteString("<conversation_history>\n")
	sb.WriteString("以下是本次会话的历史对话记录。重要指引：\n")
	sb.WriteString("1. 仔细阅读历史记录，识别已完成的诊断步骤和已采集的数据。\n")
	sb.WriteString("2. 不要重复执行已完成的操作，直接从断点处继续。\n")
	sb.WriteString("3. 向子 agent 委派任务时，必须在任务描述中说明「已完成的步骤」和「需要继续的步骤」。\n\n")
	// Keep last N turns to avoid exceeding token limits
	maxTurns := defaultConversationHistoryTurns
	start := 0
	if len(history) > maxTurns {
		start = len(history) - maxTurns
	}
	for _, msg := range history[start:] {
		content := msg.Content
		if len(content) > defaultConversationMessageChars {
			content = content[:defaultConversationMessageChars-3] + "...(truncated)"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, content))
	}
	sb.WriteString("</conversation_history>\n\n")
	sb.WriteString("[user]: ")
	sb.WriteString(message)
	return sb.String()
}

// injectMemoryContext prepends fresh memory context to the user message on each request.
// This ensures the LLM always sees the latest memory without rebuilding the agent.
// When running in scene mode (cfg.SceneID != ""), scene-scoped memory and skills are injected
// instead of user-scoped ones.
func (h *Hub) injectMemoryContext(message string, userID string) string {
	if h.memoryStore == nil && h.skillsLoader == nil {
		return message
	}

	h.runnerMu.RLock()
	sceneID := h.cfg.SceneID
	h.runnerMu.RUnlock()
	var sb strings.Builder

	if h.memoryStore != nil {
		// Shared memory (always visible)
		sharedCtx := h.memoryStore.GetMemoryContext()
		if sharedCtx != "" {
			sb.WriteString("<memory scope=\"shared\">\n")
			sb.WriteString(sharedCtx)
			sb.WriteString("</memory>\n\n")
		}

		if sceneID != "" {
			// Scene-scoped memory
			sceneCtx := h.memoryStore.ForScene(sceneID).GetMemoryContext()
			if sceneCtx != "" {
				sb.WriteString(fmt.Sprintf("<memory scope=\"scene/%s\">\n", sceneID))
				sb.WriteString(sceneCtx)
				sb.WriteString("</memory>\n\n")
			}
		} else if userID != "" && userID != "visitor" {
			// User-private memory (only in non-scene mode)
			userCtx := h.memoryStore.ForUser(userID).GetMemoryContext()
			if userCtx != "" {
				sb.WriteString(fmt.Sprintf("<memory scope=\"user/%s\">\n", userID))
				sb.WriteString(userCtx)
				sb.WriteString("</memory>\n\n")
			}
		}
	}

	// Skills injection (scene-filtered or user-filtered)
	if h.skillsLoader != nil {
		if sceneID != "" {
			// Scene mode: inject scene-visible skills
			sceneSkills := h.skillsLoader.GetAlwaysSkillsForScene(sceneID)
			if sceneSkills != "" {
				sb.WriteString("<skills scope=\"active\">\n")
				sb.WriteString(sceneSkills)
				sb.WriteString("</skills>\n\n")
			}
			skillSummary := h.skillsLoader.BuildSummaryForScene(sceneID)
			if skillSummary != "" {
				sb.WriteString("<skills scope=\"available\">\n")
				sb.WriteString(skillSummary)
				sb.WriteString("</skills>\n\n")
			}
		} else {
			// Default mode: inject user-visible skills
			effectiveUserID := userID
			if effectiveUserID == "" || effectiveUserID == "visitor" {
				effectiveUserID = ""
			}
			userSkills := h.skillsLoader.GetAlwaysSkillsForUser(effectiveUserID)
			if userSkills != "" {
				sb.WriteString("<skills scope=\"active\">\n")
				sb.WriteString(userSkills)
				sb.WriteString("</skills>\n\n")
			}
			skillSummary := h.skillsLoader.BuildSummaryForUser(effectiveUserID)
			if skillSummary != "" {
				sb.WriteString("<skills scope=\"available\">\n")
				sb.WriteString(skillSummary)
				sb.WriteString("</skills>\n\n")
			}
		}
	}

	if sb.Len() == 0 {
		return message
	}
	sb.WriteString(message)
	return sb.String()
}

func (h *Hub) ResumeIter(ctx context.Context, runID string, targets map[string]any) (*adk.AsyncIterator[*adk.AgentEvent], error) {
	return h.ResumeIterWithSession(ctx, "", runID, targets)
}

// ResumeIterWithSession is the streaming form of ResumeWithSession.
func (h *Hub) ResumeIterWithSession(ctx context.Context, sessionID, runID string, targets map[string]any) (*adk.AsyncIterator[*adk.AgentEvent], error) {
	ctx = withLLMTraceContext(ctx, sessionID, runID, "")
	if h.sandbox != nil {
		h.sandbox.CleanExpiredRunWorkspaces()
		_ = h.sandbox.RunWorkspaceDir(runID, "")
	}
	h.toolRegistry.SetContext(runID, "")
	h.runnerMu.RLock()
	runner := h.runner
	h.runnerMu.RUnlock()
	if len(targets) > 0 {
		return runner.ResumeWithParams(ctx, runID, &adk.ResumeParams{Targets: targets})
	}
	return runner.Resume(ctx, runID)
}

func (h *Hub) HasCheckpoint(runID string) bool {
	h.runnerMu.RLock()
	store := h.store
	h.runnerMu.RUnlock()
	return store.Has(runID)
}

func runPathString(ev *adk.AgentEvent) string {
	if len(ev.RunPath) == 0 {
		return ev.AgentName
	}
	parts := make([]string, 0, len(ev.RunPath))
	for _, step := range ev.RunPath {
		if s := step.String(); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ev.AgentName
	}
	return strings.Join(parts, " → ")
}

func collectResult(runID string, iter *adk.AsyncIterator[*adk.AgentEvent]) (*RunResult, error) {
	var (
		output     string
		events     []PublicEvent
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
			log.Printf("[run %s] ERROR at agent=%q path=%s: %v", runID, event.AgentName, runPathString(event), event.Err)
			return &RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts}, event.Err
		}

		// Log token usage from LLM responses.
		msg, _, _ := adk.GetMessage(event)
		if msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
			u := msg.ResponseMeta.Usage
			log.Printf("[run %s] agent=%q prompt=%d completion=%d total=%d path=%s",
				runID, event.AgentName, u.PromptTokens, u.CompletionTokens,
				u.PromptTokens+u.CompletionTokens, runPathString(event))
		}

		events = append(events, ToPublicEvent(event))
		if event.Action != nil && event.Action.Interrupted != nil {
			interrupts = event.Action.Interrupted.InterruptContexts
		}
		if msg != nil &&
			msg.Role == schema.Assistant &&
			msg.ToolCallID == "" &&
			len(msg.ToolCalls) == 0 &&
			strings.TrimSpace(msg.Content) != "" &&
			!IsStageLimitStatus(msg.Content) {
			output = msg.Content
		}
	}

	return &RunResult{RunID: runID, Output: output, Events: events, Interrupts: interrupts}, nil
}

func assembleSupervisorTools(sandboxTools []tool.BaseTool) []tool.BaseTool {
	tools := make([]tool.BaseTool, 0, 1+len(sandboxTools))
	tools = append(tools, NewApprovalTool())
	tools = append(tools, sandboxTools...)
	return tools
}

func recoverableToolsConfig(tools []tool.BaseTool) adk.ToolsConfig {
	return adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
		Tools: tools,
		UnknownToolsHandler: func(_ context.Context, name, _ string) (string, error) {
			return fmt.Sprintf(
				"工具 %q 不存在。请只使用系统实际提供的工具；如果你想更新任务进度，请直接在回复文本中输出 <!-- knsight-todos [...] --> HTML 注释，不要调用 todo_comment 或其他 todo 工具。",
				name,
			), nil
		},
	}}
}

// buildSubSupervisor creates a middle-layer agent that acts as a sub-supervisor,
// delegating to tool agents (MCP agents + external agents) as its sub-agents.
func toolAgentNames(ctx context.Context, agents []adk.Agent) []string {
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name(ctx))
	}
	return names
}

func buildSubSupervisor(ctx context.Context, baseModel model.ToolCallingChatModel, cfg AgentConfig, toolAgents []adk.Agent, sandboxTools []tool.BaseTool, ctxBuilder *ContextBuilder, recorder *llmRoundRecorder) (adk.Agent, error) {
	if cfg.Name == "" || cfg.Description == "" {
		return nil, errors.New("sub_agent name and description are required")
	}
	agentModel := baseModel
	if cfg.ModelOverride != nil {
		overrideModel, err := NewFailoverChatModel(ctx, *cfg.ModelOverride)
		if err != nil {
			return nil, err
		}
		agentModel = overrideModel
	}

	agentTools := make([]tool.BaseTool, len(sandboxTools))
	copy(agentTools, sandboxTools)
	agentTools = localToolsForSubAgent(ctx, cfg.Name, agentTools)

	instruction := ctxBuilder.BuildSubAgentPrompt(cfg, toolAgentNames(ctx, toolAgents))
	log.Printf("[hub] sub-agent %q max_iterations=%d sandbox_only=%v tool_agents=%d", cfg.Name, cfg.MaxIterations, cfg.SandboxOnly, len(toolAgents))

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Name,
		Description:   cfg.Description,
		Instruction:   instruction,
		Model:         withLLMRecording(agentModel, cfg.Name, recorder),
		ToolsConfig:   recoverableToolsConfig(agentTools),
		MaxIterations: cfg.MaxIterations,
		Middlewares:   []adk.AgentMiddleware{{BeforeChatModel: NewLoopDetector()}},
	})
	if err != nil {
		return nil, err
	}

	// Wrap as a sub-supervisor with tool agents as sub-agents. Budget exhaustion
	// returns the stage's current findings to the parent instead of aborting.
	subSupervisor, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: agent,
		SubAgents:  toolAgents,
	})
	if err != nil {
		return nil, err
	}
	budgeted := withAgentStageBudget(
		subSupervisor,
		time.Duration(cfg.TimeoutSeconds)*time.Second,
		cfg.MaxIterations,
	)
	return budgeted, nil
}

func localToolsForSubAgent(ctx context.Context, agentName string, tools []tool.BaseTool) []tool.BaseTool {
	var allowed map[string]bool
	switch agentName {
	case "InspectAgent":
		allowed = map[string]bool{
			"exec_shell":     true,
			"get_skill":      true,
			"read_memory":    true,
			"append_journal": true,
		}
	case "VisionAgent":
		allowed = map[string]bool{
			"emit_chart": true,
			"exec_shell": true,
			"read_image": true,
		}
	case "SummaryAgent":
		return nil
	default:
		return tools
	}

	filtered := make([]tool.BaseTool, 0, len(tools))
	for _, localTool := range tools {
		info, err := localTool.Info(ctx)
		if err == nil && info != nil && allowed[info.Name] {
			filtered = append(filtered, localTool)
		}
	}
	return filtered
}

func buildExternalAgent(ctx context.Context, cfg ExternalAgentConfig, fallback LLMConfig, recorder *llmRoundRecorder) (adk.Agent, error) {
	if cfg.Name == "" || cfg.Description == "" || cfg.BaseURL == "" {
		return nil, errors.New("external agent name, description, and base_url are required")
	}
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = fallback.APIKey
	}
	modelCfg := LLMConfig{
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		APIKey:  apiKey,
	}
	if modelCfg.Model == "" {
		modelCfg.Model = fallback.Model
	}
	chatModel, err := newOpenAIModel(ctx, modelCfg)
	if err != nil {
		return nil, err
	}
	// Run the same dangling-tool-call sanitizer FailoverChatModel uses, so
	// external agents whose history contains an orphan tool_use don't blow
	// up downstream Bedrock with 400 "tool_use ids were found without
	// tool_result blocks immediately after".
	wrapped := WrapSanitizing(chatModel)
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Name,
		Description:   cfg.Description,
		Instruction:   "你是外部专业代理。根据请求调用你的能力，返回结构化结果。",
		Model:         withLLMRecording(wrapped, cfg.Name, recorder),
		MaxIterations: cfg.MaxIterations,
		Middlewares:   []adk.AgentMiddleware{{BeforeChatModel: NewLoopDetector()}},
	})
}

// buildToolAgents builds all bottom-layer tool agents (MCP + external).
// ConnectMCP handles connection, isError conversion, and large-result chunking internally.
func buildToolAgents(ctx context.Context, baseModel model.ToolCallingChatModel, cfg Config, regStore *registry.Store, recorder *llmRoundRecorder) ([]adk.Agent, []*client.Client, []func(), error) {
	var agents []adk.Agent
	var clients []*client.Client
	var cleanups []func()

	// Shared rate limiter for all MCPs (applied before approval wrapper)
	var globalLimiter *MCPRateLimiter
	if cfg.Tools.GlobalMCPQPS > 0 {
		globalLimiter = NewMCPRateLimiter(cfg.Tools.GlobalMCPQPS)
		log.Printf("mcp: global rate limiter enabled (%.1f QPS)", cfg.Tools.GlobalMCPQPS)
	}

	// MCP tool agents
	for _, mcpCfg := range cfg.Tools.MCPs {
		mcpTools, cli, cleanup, err := ConnectMCP(ctx, mcpCfg)
		if err != nil {
			log.Printf("warning: mcp %q: connection failed (skipping): %v", mcpCfg.Name, err)
			continue
		}
		// len-1 because ConnectMCP appends the get_result_chunk helper tool
		log.Printf("mcp %q: loaded %d tools from %s (need_approve=%v)", mcpCfg.Name, len(mcpTools)-1, mcpCfg.Endpoint(), mcpCfg.NeedApprove)
		// Log tool schema sizes for token budget analysis
		schemaChars, toolNames := toolSchemaChars(ctx, mcpTools)
		log.Printf("mcp %q: tool schema %d chars ≈ %d tokens, tools: %v", mcpCfg.Name, schemaChars, estimateTokens(schemaChars), toolNames)

		// Apply global rate limit
		if globalLimiter != nil {
			mcpTools = WrapToolsWithRateLimit(mcpTools, globalLimiter)
		}

		// Wrap with approval if configured
		if mcpCfg.NeedApprove {
			mcpTools = WrapToolsWithApproval(mcpTools)
		}

		agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
			Name:          mcpCfg.Name,
			Description:   mcpCfg.Description,
			Instruction:   fmt.Sprintf("你是 %s 工具代理。使用你的工具完成委派的任务，直接返回结构化结果。当收到 [CHUNKED_RESULT] 标记时，使用 get_result_chunk 工具逐片获取剩余数据。", mcpCfg.Name),
			Model:         withLLMRecording(baseModel, mcpCfg.Name, recorder),
			ToolsConfig:   recoverableToolsConfig(mcpTools),
			MaxIterations: mcpCfg.MaxIterations,
			Middlewares:   []adk.AgentMiddleware{{BeforeChatModel: NewLoopDetector()}},
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("mcp %q: build agent: %w", mcpCfg.Name, err)
		}
		agents = append(agents, agent)
		clients = append(clients, cli)
		cleanups = append(cleanups, cleanup)
	}

	// External tool agents
	registryAgents, err := discoverRegistryAgents(ctx, cfg.RegistryURL, regStore)
	if err != nil {
		log.Printf("warning: registry discovery failed: %v", err)
	}
	externalCfgs := mergeExternalAgents(cfg.Tools.Agents, registryAgents)
	for _, extCfg := range externalCfgs {
		agent, err := buildExternalAgent(ctx, extCfg, cfg.LLM, recorder)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("external %q: %w", extCfg.Name, err)
		}
		agents = append(agents, agent)
		log.Printf("external agent %q: registered", extCfg.Name)
	}

	return agents, clients, cleanups, nil
}

func newOpenAIModel(ctx context.Context, cfg LLMConfig) (*openai.ChatModel, error) {
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, errors.New("llm base_url and model are required")
	}
	baseURL := normalizeBaseURL(cfg.BaseURL)
	conf := &openai.ChatModelConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		BaseURL: baseURL,
	}
	if cfg.TimeoutSeconds > 0 {
		conf.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return openai.NewChatModel(ctx, conf)
}

func normalizeBaseURL(raw string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "http://" + raw
}

func discoverRegistryAgents(ctx context.Context, registryURL string, regStore *registry.Store) ([]ExternalAgentConfig, error) {
	// If embedded registry store is available, read directly
	if regStore != nil {
		records := regStore.List()
		external := make([]ExternalAgentConfig, 0, len(records))
		for _, rec := range records {
			card := rec.Card
			protocolType := strings.ToLower(card.Protocol)
			if protocolType == "" && card.Metadata != nil {
				protocolType = strings.ToLower(card.Metadata["protocol"])
			}
			if protocolType != "openai" {
				continue
			}
			model := card.Model
			if model == "" && card.Metadata != nil {
				model = card.Metadata["model"]
			}
			external = append(external, ExternalAgentConfig{
				Name:          card.Name,
				Description:   card.Description,
				BaseURL:       card.Endpoint,
				Model:         model,
				MaxIterations: defaultAgentMaxIterations,
			})
		}
		return external, nil
	}

	// Fallback to HTTP discovery
	if registryURL == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL+"/v1/registry/agents", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry list failed: %s", resp.Status)
	}
	var records []struct {
		Card protocol.AgentCard `json:"card"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}
	external := make([]ExternalAgentConfig, 0, len(records))
	for _, rec := range records {
		card := rec.Card
		metadata := card.Metadata
		protocolType := strings.ToLower(card.Protocol)
		if protocolType == "" && metadata != nil {
			protocolType = strings.ToLower(metadata["protocol"])
		}
		if protocolType != "openai" {
			continue
		}
		model := card.Model
		if model == "" && metadata != nil {
			model = metadata["model"]
		}
		external = append(external, ExternalAgentConfig{
			Name:        card.Name,
			Description: card.Description,
			BaseURL:     card.Endpoint,
			Model:       model,
		})
	}
	return external, nil
}

// estimateTokens roughly estimates token count from character length (1 token ≈ 3.5 chars for mixed en/zh).
func estimateTokens(chars int) int {
	return (chars*10 + 34) / 35 // ≈ chars / 3.5, integer math
}

// toolSchemaChars returns the approximate character count of all tool schemas.
func toolSchemaChars(ctx context.Context, tools []tool.BaseTool) (int, []string) {
	total := 0
	var names []string
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		names = append(names, info.Name)
		// Name + description + params (marshal the whole ToolInfo for size estimate)
		raw, _ := json.Marshal(info)
		total += len(raw)
	}
	return total, names
}

// logTokenBudget prints a detailed breakdown of estimated token consumption for each agent layer.
func logTokenBudget(ctx context.Context, cfg Config, supervisorInstruction string, supervisorTools []tool.BaseTool, toolAgents []adk.Agent, sandboxTools []tool.BaseTool, ctxBuilder *ContextBuilder, memStore *memory.MemoryStore) {
	log.Printf("=== Token Budget Estimation ===")

	// Memory context
	memChars := 0
	if memStore != nil {
		memCtx := memStore.GetMemoryContext()
		memChars = len(memCtx)
		log.Printf("  memory context: %d chars ≈ %d tokens", memChars, estimateTokens(memChars))
	}

	// Supervisor
	supInstrChars := len(supervisorInstruction)
	supToolChars, supToolNames := toolSchemaChars(ctx, supervisorTools)
	log.Printf("  supervisor [%s]:", cfg.Supervisor.Name)
	log.Printf("    instruction: %d chars ≈ %d tokens", supInstrChars, estimateTokens(supInstrChars))
	log.Printf("    tools: %d tools, schema %d chars ≈ %d tokens", len(supervisorTools), supToolChars, estimateTokens(supToolChars))
	log.Printf("    tool names: %v", supToolNames)

	// Tool agents — their MCP tool schemas are logged at build time (see buildToolAgents)
	for _, ta := range toolAgents {
		log.Printf("  tool-agent [%s]: (sub-agent of each sub-supervisor, schema logged above)", ta.Name(ctx))
	}

	// Sandbox tools
	sbToolChars, sbToolNames := toolSchemaChars(ctx, sandboxTools)
	log.Printf("  sandbox tools: %d tools, schema %d chars ≈ %d tokens", len(sandboxTools), sbToolChars, estimateTokens(sbToolChars))
	log.Printf("    tool names: %v", sbToolNames)

	// Sub-agents
	taNames := toolAgentNames(ctx, toolAgents)
	totalSubAgent := 0
	for _, agentCfg := range cfg.SubAgents {
		instrText := ctxBuilder.BuildSubAgentPrompt(agentCfg, taNames)
		instrChars := len(instrText)

		// Each sub-supervisor gets: its own instruction + sandbox tools + all tool agents as sub-agents
		// The tool agents each carry their own MCP tools + instruction
		agentTotal := instrChars + sbToolChars
		log.Printf("  sub-agent [%s]:", agentCfg.Name)
		log.Printf("    instruction: %d chars ≈ %d tokens", instrChars, estimateTokens(instrChars))
		log.Printf("    own tools (sandbox): %d tools, %d chars ≈ %d tokens", len(sandboxTools), sbToolChars, estimateTokens(sbToolChars))
		log.Printf("    sub-agent total (excl. tool-agent schemas): %d chars ≈ %d tokens", agentTotal, estimateTokens(agentTotal))
		totalSubAgent += agentTotal
	}

	// Grand total estimate (rough upper bound for deepest call path)
	// Worst case: supervisor instruction + sub-agent instruction + tool-agent tools + sandbox tools + memory
	grandTotal := supInstrChars + supToolChars + totalSubAgent + memChars
	log.Printf("  ---")
	log.Printf("  estimated total (all layers combined): %d chars ≈ %d tokens", grandTotal, estimateTokens(grandTotal))
	log.Printf("  note: actual token count depends on tool-agent MCP schemas (not measured here) + conversation history")
	log.Printf("=== End Token Budget ===")
}

func mergeExternalAgents(explicit []ExternalAgentConfig, discovered []ExternalAgentConfig) []ExternalAgentConfig {
	seen := make(map[string]bool)
	result := make([]ExternalAgentConfig, 0, len(explicit)+len(discovered))
	for _, agent := range explicit {
		if agent.Name == "" {
			continue
		}
		seen[strings.ToLower(agent.Name)] = true
		result = append(result, agent)
	}
	for _, agent := range discovered {
		if agent.Name == "" {
			continue
		}
		key := strings.ToLower(agent.Name)
		if seen[key] {
			continue
		}
		result = append(result, agent)
		seen[key] = true
	}
	return result
}
