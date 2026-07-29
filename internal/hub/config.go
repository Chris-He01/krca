package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultRateLimitMaxRetries      = 5
	defaultRateLimitWaitSeconds     = 30
	defaultContextCompactMaxRetries = 2
	defaultContextCompactTarget     = 0.55
	defaultAgentMaxIterations       = 300
	defaultSandboxMaxOutputBytes    = 20_000
	defaultConversationHistoryTurns = 10
	defaultConversationMessageChars = 1500
)

type Config struct {
	ListenAddr   string `json:"listen_addr" yaml:"listen_addr"`
	RegistryURL  string `json:"registry_url" yaml:"registry_url"`
	ProfileToken string `json:"profile_token,omitempty" yaml:"profile_token,omitempty"`

	LLM   LLMConfig   `json:"llm" yaml:"llm"`
	Tools ToolsConfig `json:"tools" yaml:"tools"`

	// UserSelectableModels lists the model options shown in the chat UI.
	// Populated with built-in defaults in ApplyDefaults when empty.
	UserSelectableModels []UserSelectableModel `json:"user_selectable_models,omitempty" yaml:"user_selectable_models,omitempty"`

	RunLimitProfiles []RunLimitProfile `json:"run_limit_profiles,omitempty" yaml:"run_limit_profiles,omitempty"`

	Supervisor AgentConfig   `json:"supervisor" yaml:"supervisor"`
	SubAgents  []AgentConfig `json:"sub_agents" yaml:"sub_agents"`

	Sandbox SandboxConfig `json:"sandbox" yaml:"sandbox"`
	Memory  MemoryConfig  `json:"memory" yaml:"memory"`
	Skills  SkillsConfig  `json:"skills" yaml:"skills"`
	Store   StoreConfig   `json:"store" yaml:"store"`
	Redis   RedisConfig   `json:"redis" yaml:"redis"`
	Kim     KimConfig     `json:"kim" yaml:"kim"`
	Log     LogConfig     `json:"log" yaml:"log"`
	Auth    AuthConfig    `json:"auth" yaml:"auth"`
	CK      CKConfig      `json:"ck" yaml:"ck"`

	// SceneID is the active scene identifier, set from KNSIGHT_SCENE env var.
	// When non-empty, the Hub loads scene-specific config that overrides
	// Supervisor, SubAgents, Tools, and Skills visibility.
	SceneID string `json:"scene_id,omitempty" yaml:"scene_id,omitempty"`
}

type AuthConfig struct {
	// Enabled is the master toggle. When false, all requests pass through as
	// visitor regardless of mode. Pointer so we can distinguish "unset"
	// (default to true) from explicit false. Override via env
	// KNSIGHT_AUTH_ENABLED=true|false.
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Mode selects the authentication backend:
	//   ""            - back-compat: cookie+SSO if SSORequired, else disabled
	//   "disabled"    - no enforcement; all requests pass as visitor
	//   "cookie"      - knsight_user_id cookie + SSO redirect (legacy)
	//   "accessproxy" - AccessProxy identity-passthrough JWT (header X-Identity-Token)
	//   "auto"        - try AccessProxy first; fall back to cookie when token absent/invalid
	// Override via env KNSIGHT_AUTH_MODE.
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`

	// SSORequired controls whether unauthenticated users are redirected to SSO.
	// Only meaningful for cookie/auto modes. Override via env KNSIGHT_SSO_REQUIRED.
	SSORequired bool `json:"sso_required" yaml:"sso_required"`
	// SSOURL is the SSO login page. Users are redirected here when SSORequired=true and no cookie is found.
	SSOURL string `json:"sso_url" yaml:"sso_url"`

	// AccessProxy holds settings for the generic identity-token client.
	// Required when Mode is "accessproxy" or "auto".
	AccessProxy AccessProxyConfig `json:"access_proxy,omitempty" yaml:"access_proxy,omitempty"`
}

// AccessProxyConfig mirrors user.AccessProxyConfig in YAML/JSON form so the
// hub package stays free of the SDK import.
type AccessProxyConfig struct {
	// JwksURL is the AP-published JWKS endpoint. Empty => use SDK default.
	JwksURL string `json:"jwks_url,omitempty" yaml:"jwks_url,omitempty"`
	// PublicHost is retained for configuration compatibility and identifies the public host.
	// Used by AP for adoption telemetry only.
	PublicHost string `json:"public_host,omitempty" yaml:"public_host,omitempty"`
	// VerifyIss verifies the token issuer matches the AP node IP. Enable only
	// when this service is AP's first hop (no intermediate proxy).
	VerifyIss bool `json:"verify_iss,omitempty" yaml:"verify_iss,omitempty"`
	// TrustedHosts restricts accepted token `host` claims; empty list allows any.
	TrustedHosts []string `json:"trusted_hosts,omitempty" yaml:"trusted_hosts,omitempty"`
	// TrustedUpstreams restricts accepted token `upstream_id` claims; empty allows any.
	TrustedUpstreams []int64 `json:"trusted_upstreams,omitempty" yaml:"trusted_upstreams,omitempty"`
}

type KimConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	APIKey  string `json:"api_key" yaml:"api_key"`   // Kim robot key, e.g. "f9a975c3-..."
	BaseURL string `json:"base_url" yaml:"base_url"` // optional webhook endpoint
}

type LogConfig struct {
	Level      string `json:"level" yaml:"level"`             // debug, info, warn, error
	File       string `json:"file" yaml:"file"`               // log file path, empty = stdout only
	MaxSizeMB  int    `json:"max_size_mb" yaml:"max_size_mb"` // max size per file in MB
	MaxBackups int    `json:"max_backups" yaml:"max_backups"` // max number of old files
	MaxAgeDays int    `json:"max_age_days" yaml:"max_age_days"`
	Compress   bool   `json:"compress" yaml:"compress"`
}

type LLMConfig struct {
	BaseURL                  string  `json:"base_url" yaml:"base_url"`
	Model                    string  `json:"model" yaml:"model"` // single model or comma-separated for round-robin
	APIKey                   string  `json:"api_key" yaml:"api_key"`
	MaxTokens                int     `json:"max_tokens" yaml:"max_tokens"`
	ContextWindow            int     `json:"context_window,omitempty" yaml:"context_window,omitempty"`
	ContextCompactEnabled    *bool   `json:"context_compact_enabled,omitempty" yaml:"context_compact_enabled,omitempty"`
	ContextCompactTarget     float64 `json:"context_compact_target,omitempty" yaml:"context_compact_target,omitempty"`
	ContextCompactMaxRetries int     `json:"context_compact_max_retries,omitempty" yaml:"context_compact_max_retries,omitempty"`
	TimeoutSeconds           int     `json:"timeout_seconds" yaml:"timeout_seconds"`
	RateLimitMaxRetries      int     `json:"rate_limit_max_retries" yaml:"rate_limit_max_retries"`
	RateLimitWaitSeconds     int     `json:"rate_limit_wait_seconds" yaml:"rate_limit_wait_seconds"`
}

// UserSelectableModel describes one model option exposed to the user in the chat UI.
// Label is the display name shown in the dropdown; ModelID is the actual model identifier
// sent to the LLM endpoint. When ModelID is empty the system default (cfg.LLM) is used.
type UserSelectableModel struct {
	Label   string `json:"label" yaml:"label"`
	ModelID string `json:"model_id" yaml:"model_id"`
	// Override fields — when non-empty they replace the corresponding field from cfg.LLM.
	BaseURL   string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
}

// RunLimitProfile is a user-selectable execution budget. PreserveConfigured
// keeps each agent's YAML values; otherwise the values are minimum limits.
type RunLimitProfile struct {
	ID                 string `json:"id" yaml:"id"`
	Label              string `json:"label" yaml:"label"`
	Description        string `json:"description,omitempty" yaml:"description,omitempty"`
	PreserveConfigured bool   `json:"preserve_configured,omitempty" yaml:"preserve_configured,omitempty"`
	MaxIterations      int    `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
	TimeoutSeconds     int    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
}

// UnmarshalYAML accepts max_token as a backwards-compatible alias for max_tokens.
func (m *UserSelectableModel) UnmarshalYAML(value *yaml.Node) error {
	type plain UserSelectableModel
	var raw struct {
		Plain    plain `yaml:",inline"`
		MaxToken int   `yaml:"max_token"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*m = UserSelectableModel(raw.Plain)
	if m.MaxTokens == 0 {
		m.MaxTokens = raw.MaxToken
	}
	return nil
}

// IsDefault returns true when this entry represents the built-in system model (no override).
func (m UserSelectableModel) IsDefault() bool {
	return m.ModelID == "" || m.ModelID == "Knsight"
}

// Models returns the list of models parsed from the comma-separated Model field.
func (c LLMConfig) Models() []string {
	parts := strings.Split(c.Model, ",")
	var models []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			models = append(models, p)
		}
	}
	return models
}

func (c LLMConfig) rateLimitMaxRetries() int {
	if c.RateLimitMaxRetries < 0 {
		return 0
	}
	if c.RateLimitMaxRetries == 0 {
		return defaultRateLimitMaxRetries
	}
	return c.RateLimitMaxRetries
}

func (c LLMConfig) rateLimitWaitSeconds() int {
	if c.RateLimitWaitSeconds <= 0 {
		return defaultRateLimitWaitSeconds
	}
	return c.RateLimitWaitSeconds
}

func (c LLMConfig) contextCompactEnabled() bool {
	return c.ContextCompactEnabled == nil || *c.ContextCompactEnabled
}

func (c LLMConfig) contextCompactTarget() float64 {
	if c.ContextCompactTarget <= 0 || c.ContextCompactTarget >= 1 {
		return defaultContextCompactTarget
	}
	return c.ContextCompactTarget
}

func (c LLMConfig) contextCompactMaxRetries() int {
	if c.ContextCompactMaxRetries < 0 {
		return 0
	}
	if c.ContextCompactMaxRetries == 0 {
		return defaultContextCompactMaxRetries
	}
	return c.ContextCompactMaxRetries
}

type ToolsConfig struct {
	MCPs                      []MCPConfig           `json:"mcps" yaml:"mcps"`
	Agents                    []ExternalAgentConfig `json:"agents" yaml:"agents"`
	GlobalMCPQPS              float64               `json:"global_mcp_qps" yaml:"global_mcp_qps"` // max calls/sec across all MCPs; 0 = unlimited
	DefaultAgentMaxIterations int                   `json:"default_agent_max_iterations" yaml:"default_agent_max_iterations"`
}

type MCPConfig struct {
	Name          string `json:"name" yaml:"name"`
	Description   string `json:"description" yaml:"description"`
	SSEURL        string `json:"sse_url,omitempty" yaml:"sse_url,omitempty"` // SSE transport endpoint
	URL           string `json:"url,omitempty" yaml:"url,omitempty"`         // Streamable HTTP transport endpoint
	APIKey        string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	SignTokenURL  string `json:"sign_token_url,omitempty" yaml:"sign_token_url,omitempty"` // per-user token signing endpoint
	NeedApprove   bool   `json:"need_approve" yaml:"need_approve"`                         // default false
	MaxIterations int    `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
}

// IsStreamableHTTP returns true if this MCP should use Streamable HTTP transport.
func (c MCPConfig) IsStreamableHTTP() bool {
	return c.URL != ""
}

// Endpoint returns the primary endpoint URL (url takes precedence over sse_url).
func (c MCPConfig) Endpoint() string {
	if c.URL != "" {
		return c.URL
	}
	return c.SSEURL
}

type AgentConfig struct {
	Name           string     `json:"name" yaml:"name"`
	Description    string     `json:"description" yaml:"description"`
	Instruction    string     `json:"instruction" yaml:"instruction"`
	InstructionRef string     `json:"instruction_ref,omitempty" yaml:"instruction_ref,omitempty"` // path to prompt file; if set, file content replaces Instruction
	ModelOverride  *LLMConfig `json:"model_override,omitempty" yaml:"model_override,omitempty"`
	MaxIterations  int        `json:"max_iterations" yaml:"max_iterations"`
	TimeoutSeconds int        `json:"timeout_seconds" yaml:"timeout_seconds"`
	// SandboxOnly disallows delegation to external tool agents; only local sandbox tools are available.
	SandboxOnly bool `json:"sandbox_only" yaml:"sandbox_only"`
}

// ResolveInstruction loads the prompt file specified by InstructionRef (if set)
// and replaces the Instruction field with its content.
func (c *AgentConfig) ResolveInstruction() error {
	if c.InstructionRef == "" {
		return nil
	}
	data, err := os.ReadFile(c.InstructionRef)
	if err != nil {
		return fmt.Errorf("load instruction_ref %q for agent %q: %w", c.InstructionRef, c.Name, err)
	}
	c.Instruction = string(data)
	return nil
}

// SceneConfig holds scene-specific overrides loaded from a scene config file.
// When KNSIGHT_SCENE is set, the corresponding scene config is loaded and
// merged into the base Config, overriding Supervisor, SubAgents, and Tools.
type SceneConfig struct {
	ID         string        `json:"id" yaml:"id"`
	Supervisor AgentConfig   `json:"supervisor" yaml:"supervisor"`
	SubAgents  []AgentConfig `json:"sub_agents" yaml:"sub_agents"`
	Tools      ToolsConfig   `json:"tools,omitempty" yaml:"tools,omitempty"`
	Runtime    struct {
		TotalTimeoutSec int `json:"total_timeout_sec,omitempty" yaml:"total_timeout_sec,omitempty"`
	} `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}

// LoadSceneConfig reads a scene config YAML file.
func LoadSceneConfig(path string) (SceneConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SceneConfig{}, fmt.Errorf("read scene config %q: %w", path, err)
	}
	data = []byte(os.ExpandEnv(string(data)))

	var sc SceneConfig
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return SceneConfig{}, fmt.Errorf("parse scene config %q: %w", path, err)
	}
	return sc, nil
}

// ApplyScene merges a SceneConfig into the base Config, overriding
// Supervisor, SubAgents, Tools, and runtime parameters. It also resolves
// InstructionRef for all agents that reference external prompt files.
func (c *Config) ApplyScene(sc SceneConfig) error {
	c.SceneID = sc.ID

	if sc.Supervisor.Name != "" {
		c.Supervisor = sc.Supervisor
	}
	if len(sc.SubAgents) > 0 {
		c.SubAgents = sc.SubAgents
	}
	if len(sc.Tools.MCPs) > 0 || len(sc.Tools.Agents) > 0 {
		c.Tools = sc.Tools
	}
	if sc.Runtime.TotalTimeoutSec > 0 {
		if c.Supervisor.TimeoutSeconds == 0 {
			c.Supervisor.TimeoutSeconds = sc.Runtime.TotalTimeoutSec
		}
		for i := range c.SubAgents {
			if c.SubAgents[i].TimeoutSeconds == 0 {
				c.SubAgents[i].TimeoutSeconds = sc.Runtime.TotalTimeoutSec
			}
		}
	}

	// Resolve instruction_ref for all agents
	if err := c.Supervisor.ResolveInstruction(); err != nil {
		return err
	}
	for i := range c.SubAgents {
		if err := c.SubAgents[i].ResolveInstruction(); err != nil {
			return err
		}
	}

	// Re-apply defaults for any zero-valued fields after override
	c.ApplyDefaults()
	return nil
}

type ExternalAgentConfig struct {
	Name          string `json:"name" yaml:"name"`
	Description   string `json:"description" yaml:"description"`
	BaseURL       string `json:"base_url" yaml:"base_url"`
	Model         string `json:"model" yaml:"model"`
	APIKey        string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
}

type SandboxConfig struct {
	Enabled              bool     `json:"enabled" yaml:"enabled"`
	AutoApprove          *bool    `json:"auto_approve" yaml:"auto_approve"` // nil = true (default auto)
	WorkspaceDir         string   `json:"workspace_dir" yaml:"workspace_dir"`
	DenyPatterns         []string `json:"deny_patterns" yaml:"deny_patterns"`
	MaxOutputBytes       int      `json:"max_output_bytes" yaml:"max_output_bytes"`
	CommandTimeoutSec    int      `json:"command_timeout_seconds" yaml:"command_timeout_seconds"`
	RestrictToWorkspace  bool     `json:"restrict_to_workspace" yaml:"restrict_to_workspace"`
	SessionBaseDir       string   `json:"session_base_dir" yaml:"session_base_dir"`
	SessionMaxAgeSec     int      `json:"session_max_age_seconds" yaml:"session_max_age_seconds"`
	WebFetchEnabled      bool     `json:"web_fetch_enabled" yaml:"web_fetch_enabled"`
	WebFetchTimeoutSec   int      `json:"web_fetch_timeout_seconds" yaml:"web_fetch_timeout_seconds"`
	WebFetchMaxBodyBytes int      `json:"web_fetch_max_body_bytes" yaml:"web_fetch_max_body_bytes"`
}

type MemoryConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	WorkspaceDir string `json:"workspace_dir" yaml:"workspace_dir"`
	RecentDays   int    `json:"recent_days" yaml:"recent_days"`
	MaxMessages  int    `json:"max_messages" yaml:"max_messages"`
}

type SkillsConfig struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"`
	SkillDir        string `json:"skill_dir" yaml:"skill_dir"`
	DefaultSkillDir string `json:"default_skill_dir" yaml:"default_skill_dir"` // bundled skills dir, loaded alongside skill_dir
	DataDir         string `json:"data_dir" yaml:"data_dir"`
}

type StoreConfig struct {
	// Backend selects the storage engine: "sqlite" (default) or "redis".
	// When "redis", all session/message/snapshot data goes to Redis only.
	// When "sqlite", all data is stored in the local SQLite DB.
	Backend string `json:"backend" yaml:"backend"`
	DBPath  string `json:"db_path" yaml:"db_path"` // SQLite DB file path; only used when backend=sqlite
}

// UseRedis returns true when the Redis storage backend is selected.
func (s StoreConfig) UseRedis() bool {
	return s.Backend == "redis"
}

// CKConfig wires the ClickHouse HTTP gateway used by the /v1/tools/ck/query
// endpoint. The gateway is the same one consumed by sysobservable-alert-manager
// (themis-olap-gateway). Auth is identity-passthrough via Ks-Auth-* headers.
//
// Env overrides — picked up by ApplyDefaults — let prod deploys keep the
// token out of YAML:
//
//	KNSIGHT_CK_ENABLED   = "true" | "false"
//	KNSIGHT_CK_GATEWAY   = "https://analytics.example.com"
//	KNSIGHT_CK_TOKEN     = "<Ks-Auth-Token>"
//	KNSIGHT_CK_USER      = "liusi"
//	KNSIGHT_CK_PRINCIPAL = "service-account"
type CKConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	GatewayURL   string `json:"gateway_url" yaml:"gateway_url"`
	Token        string `json:"token" yaml:"token"`
	User         string `json:"user" yaml:"user"`
	Principal    string `json:"principal" yaml:"principal"`
	AuthType     string `json:"auth_type" yaml:"auth_type"`
	TimeoutSec   int    `json:"timeout_seconds" yaml:"timeout_seconds"`
	DefaultLimit int    `json:"default_limit" yaml:"default_limit"`
	MaxRows      int    `json:"max_rows" yaml:"max_rows"`
}

type RedisConfig struct {
	ResourceName string `json:"resource_name" yaml:"resource_name"` // kedis resource name for service discovery
	Prefix       string `json:"prefix" yaml:"prefix"`               // key prefix, e.g. "prod"
}

func (c SandboxConfig) IsAutoApprove() bool {
	return c.AutoApprove == nil || *c.AutoApprove
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// Expand environment variables (e.g., ${LLM_API_KEY}) in config content
	data = []byte(os.ExpandEnv(string(data)))

	var cfg Config
	// Check if filename contains .yaml or .yml anywhere.
	name := strings.ToLower(filepath.Base(path))
	isYAML := strings.Contains(name, ".yaml") || strings.Contains(name, ".yml")
	if isYAML {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	}

	cfg.ApplyDefaults()
	if maxTokens := strings.TrimSpace(os.Getenv("LLM_MAX_TOKENS")); maxTokens != "" {
		n, err := strconv.Atoi(maxTokens)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("LLM_MAX_TOKENS must be a positive integer, got %q", maxTokens)
		}
		cfg.LLM.MaxTokens = n
	}

	// Resolve instruction_ref for all agents (supports scene configs loaded directly via HUB_CONFIG)
	if err := cfg.Supervisor.ResolveInstruction(); err != nil {
		return Config{}, err
	}
	for i := range cfg.SubAgents {
		if err := cfg.SubAgents[i].ResolveInstruction(); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

func (c *Config) ApplyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "http://localhost:8090/v1"
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "mock"
	}
	if c.LLM.APIKey == "" {
		c.LLM.APIKey = "mock"
	}
	if len(c.UserSelectableModels) == 0 {
		c.UserSelectableModels = []UserSelectableModel{
			{
				Label:   "Knsight",
				ModelID: "Knsight",
			},
			{
				Label:   "Qwen3.6-27B",
				ModelID: "Qwen/Qwen3.6-27B",
				BaseURL: "http://localhost:8090/v1",
			},
		}
	}
	if len(c.RunLimitProfiles) == 0 {
		c.RunLimitProfiles = []RunLimitProfile{
			{
				ID:                 "standard",
				Label:              "当前限制",
				Description:        "使用各 Agent 当前配置的轮数和超时限制",
				PreserveConfigured: true,
			},
			{
				ID:             "extended",
				Label:          "超长限制",
				Description:    "复杂问题使用，至少 300 轮、3600 秒",
				MaxIterations:  300,
				TimeoutSeconds: 3600,
			},
		}
	}
	if c.Supervisor.Name == "" {
		c.Supervisor.Name = "InsightSupervisor"
	}
	if c.Supervisor.Description == "" {
		c.Supervisor.Description = "Top-level coordinator for RCA workflows."
	}
	if c.Supervisor.Instruction == "" {
		c.Supervisor.Instruction = "You are the supervisor. Delegate tasks to sub agents and summarize results.\n\n" +
			"At the start of each task and after each completed step, output a plan progress block:\n" +
			"<!-- knsight-todos [{\"id\":1,\"content\":\"Step description\",\"status\":\"in_progress\"}, ...] -->\n" +
			"This is a plain-text HTML comment, not a tool. Never call todo_comment, knsight-todos, or any todo tool.\n" +
			"Update status to \"completed\" when a step is done, \"in_progress\" for the current step, \"pending\" for future steps.\n" +
			"This block is stripped from the user-facing output automatically."
	}
	if c.Supervisor.MaxIterations == 0 {
		c.Supervisor.MaxIterations = 300
	}
	if c.Supervisor.TimeoutSeconds == 0 {
		c.Supervisor.TimeoutSeconds = 300
	}
	for i := range c.SubAgents {
		if c.SubAgents[i].MaxIterations == 0 {
			c.SubAgents[i].MaxIterations = 300
		}
		if c.SubAgents[i].TimeoutSeconds == 0 {
			c.SubAgents[i].TimeoutSeconds = 300
		}
	}
	if c.Tools.DefaultAgentMaxIterations == 0 {
		c.Tools.DefaultAgentMaxIterations = defaultAgentMaxIterations
	}
	for i := range c.Tools.MCPs {
		if c.Tools.MCPs[i].MaxIterations == 0 {
			c.Tools.MCPs[i].MaxIterations = c.Tools.DefaultAgentMaxIterations
		}
	}
	for i := range c.Tools.Agents {
		if c.Tools.Agents[i].MaxIterations == 0 {
			c.Tools.Agents[i].MaxIterations = c.Tools.DefaultAgentMaxIterations
		}
	}

	// Sandbox defaults
	if c.Sandbox.MaxOutputBytes == 0 {
		c.Sandbox.MaxOutputBytes = defaultSandboxMaxOutputBytes
	}
	if c.Sandbox.CommandTimeoutSec == 0 {
		c.Sandbox.CommandTimeoutSec = 30
	}
	if c.Sandbox.SessionBaseDir == "" {
		c.Sandbox.SessionBaseDir = "sandbox/sessions"
	}
	if c.Sandbox.SessionMaxAgeSec == 0 {
		c.Sandbox.SessionMaxAgeSec = 3600
	}
	if c.Sandbox.WebFetchTimeoutSec == 0 {
		c.Sandbox.WebFetchTimeoutSec = 30
	}
	if c.Sandbox.WebFetchMaxBodyBytes == 0 {
		c.Sandbox.WebFetchMaxBodyBytes = 500_000
	}

	// Memory defaults
	if c.Memory.RecentDays == 0 {
		c.Memory.RecentDays = 7
	}
	if c.Memory.MaxMessages == 0 {
		c.Memory.MaxMessages = 50
	}

	// Log defaults
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.MaxSizeMB == 0 {
		c.Log.MaxSizeMB = 100
	}
	if c.Log.MaxBackups == 0 {
		c.Log.MaxBackups = 5
	}
	if c.Log.MaxAgeDays == 0 {
		c.Log.MaxAgeDays = 30
	}

	// Skills defaults
	if c.Skills.SkillDir == "" {
		c.Skills.SkillDir = "skills"
	}
	if c.Skills.DataDir == "" {
		c.Skills.DataDir = "data"
	}

	// Store defaults
	if c.Store.Backend == "" {
		c.Store.Backend = "sqlite"
	}
	if c.Store.DBPath == "" {
		c.Store.DBPath = "store/knsight.db"
	}

	// CK defaults + env overrides. Env wins over file so prod can keep the
	// token out of the YAML repo.
	if v := os.Getenv("KNSIGHT_CK_ENABLED"); v != "" {
		c.CK.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("KNSIGHT_CK_GATEWAY"); v != "" {
		c.CK.GatewayURL = v
	}
	if v := os.Getenv("KNSIGHT_CK_TOKEN"); v != "" {
		c.CK.Token = v
	}
	if v := os.Getenv("KNSIGHT_CK_USER"); v != "" {
		c.CK.User = v
	}
	if v := os.Getenv("KNSIGHT_CK_PRINCIPAL"); v != "" {
		c.CK.Principal = v
	}
	if c.CK.AuthType == "" {
		c.CK.AuthType = "USER"
	}
	if c.CK.TimeoutSec == 0 {
		c.CK.TimeoutSec = 300
	}
	if c.CK.DefaultLimit == 0 {
		c.CK.DefaultLimit = 1000
	}
	if c.CK.MaxRows == 0 {
		c.CK.MaxRows = 10000
	}
}
