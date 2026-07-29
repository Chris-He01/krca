package hub

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// ConfigStore is the interface for reading/writing config settings to a persistent store.
type ConfigStore interface {
	UpsertConfigSetting(key, value string) error
	GetConfigSetting(key string) (string, error)
	GetAllConfigSettings() (map[string]string, error)
}

// ConfigAPI provides HTTP handlers for config management.
// It also implements ConfigWatcher so the Hub can do fast change detection.
type ConfigAPI struct {
	configPath string
	store      ConfigStore // optional: if set, DB config takes precedence
	version    atomic.Int64
}

// NewConfigAPI creates a new ConfigAPI backed by YAML only.
func NewConfigAPI(configPath string) *ConfigAPI {
	return &ConfigAPI{configPath: configPath}
}

// NewConfigAPIWithStore creates a new ConfigAPI that uses a DB store for persistence.
func NewConfigAPIWithStore(configPath string, store ConfigStore) *ConfigAPI {
	return &ConfigAPI{configPath: configPath, store: store}
}

// SeedDBFromConfig seeds the DB with the current config file if the DB has no agent settings.
// This is called once on startup.
func (c *ConfigAPI) SeedDBFromConfig(cfg *Config) {
	if c.store == nil {
		return
	}
	// Check if agents key already exists in DB
	existing, err := c.store.GetConfigSetting("agents")
	if err == nil && existing != "" {
		return // DB already has config, don't overwrite
	}

	// Seed DB from the config
	if err := c.writeConfigToDB(cfg); err != nil {
		log.Printf("[config] seed DB error: %v", err)
	} else {
		log.Printf("[config] seeded DB from config file")
	}
}

// GetConfig handles GET /v1/config - returns sanitized config (API keys masked).
func (c *ConfigAPI) GetConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, cfg)
	}
}

// UpdateConfig handles PUT /v1/config - accepts full Config JSON, writes to YAML and DB.
func (c *ConfigAPI) UpdateConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var updated Config
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := c.writeConfig(&updated); err != nil {
			log.Printf("[config] write error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, updated)
	}
}

// GetAgents handles GET /v1/config/agents - returns supervisor + sub_agents.
func (c *ConfigAPI) GetAgents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, map[string]any{
			"supervisor": cfg.Supervisor,
			"sub_agents": cfg.SubAgents,
		})
	}
}

// UpdateAgents handles PUT /v1/config/agents - update supervisor + sub_agents.
func (c *ConfigAPI) UpdateAgents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var body struct {
			Supervisor AgentConfig   `json:"supervisor"`
			SubAgents  []AgentConfig `json:"sub_agents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cfg.Supervisor = body.Supervisor
		cfg.SubAgents = body.SubAgents

		if err := c.writeConfig(&cfg); err != nil {
			log.Printf("[config] write error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, map[string]any{
			"supervisor": cfg.Supervisor,
			"sub_agents": cfg.SubAgents,
		})
	}
}

// GetMemoryConfig handles GET /v1/config/memory.
func (c *ConfigAPI) GetMemoryConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, cfg.Memory)
	}
}

// UpdateMemoryConfig handles PUT /v1/config/memory.
func (c *ConfigAPI) UpdateMemoryConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var mem MemoryConfig
		if err := json.NewDecoder(r.Body).Decode(&mem); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.Memory = mem

		if err := c.writeConfig(&cfg); err != nil {
			log.Printf("[config] write error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, cfg.Memory)
	}
}

// GetSandboxConfig handles GET /v1/config/sandbox.
func (c *ConfigAPI) GetSandboxConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, cfg.Sandbox)
	}
}

// UpdateSandboxConfig handles PUT /v1/config/sandbox.
func (c *ConfigAPI) UpdateSandboxConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg, err := c.readConfig()
		if err != nil {
			log.Printf("[config] read error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var sb SandboxConfig
		if err := json.NewDecoder(r.Body).Decode(&sb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.Sandbox = sb

		if err := c.writeConfig(&cfg); err != nil {
			log.Printf("[config] write error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeConfigJSON(w, cfg.Sandbox)
	}
}

// HandleConfig is a combined handler for /v1/config (no trailing slash).
func (c *ConfigAPI) HandleConfig() http.HandlerFunc {
	get := c.GetConfig()
	put := c.UpdateConfig()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			get(w, r)
		case http.MethodPut:
			put(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// HandleConfigSubpath is a combined handler for /v1/config/{subpath}.
func (c *ConfigAPI) HandleConfigSubpath() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/v1/config/")
		switch sub {
		case "agents":
			switch r.Method {
			case http.MethodGet:
				c.GetAgents()(w, r)
			case http.MethodPut:
				c.UpdateAgents()(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "memory":
			switch r.Method {
			case http.MethodGet:
				c.GetMemoryConfig()(w, r)
			case http.MethodPut:
				c.UpdateMemoryConfig()(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "sandbox":
			switch r.Method {
			case http.MethodGet:
				c.GetSandboxConfig()(w, r)
			case http.MethodPut:
				c.UpdateSandboxConfig()(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			http.NotFound(w, r)
		}
	}
}

// readConfig reads the config, merging YAML base with DB overrides.
func (c *ConfigAPI) readConfig() (Config, error) {
	// Start with YAML config as base
	cfg, err := LoadConfig(c.configPath)
	if err != nil {
		return Config{}, err
	}

	// Apply DB overrides if store is configured
	if c.store != nil {
		c.applyDBOverrides(&cfg)
	}

	return cfg, nil
}

// applyDBOverrides merges DB config settings on top of the YAML config.
func (c *ConfigAPI) applyDBOverrides(cfg *Config) {
	if v, err := c.store.GetConfigSetting("agents"); err == nil && v != "" {
		var agents struct {
			Supervisor AgentConfig   `json:"supervisor"`
			SubAgents  []AgentConfig `json:"sub_agents"`
		}
		if err := json.Unmarshal([]byte(v), &agents); err == nil {
			if agents.Supervisor.Name != "" {
				cfg.Supervisor = agents.Supervisor
			}
			if len(agents.SubAgents) > 0 {
				cfg.SubAgents = agents.SubAgents
			}
		}
	}

	if v, err := c.store.GetConfigSetting("memory"); err == nil && v != "" {
		var mem MemoryConfig
		if err := json.Unmarshal([]byte(v), &mem); err == nil {
			cfg.Memory = mem
		}
	}

	if v, err := c.store.GetConfigSetting("sandbox"); err == nil && v != "" {
		var sb SandboxConfig
		if err := json.Unmarshal([]byte(v), &sb); err == nil {
			cfg.Sandbox = sb
		}
	}
}

// Version returns the current config version number.
// Incremented atomically on every successful write — just an atomic load (~1 ns).
// Implements ConfigWatcher.
func (c *ConfigAPI) Version() int64 {
	return c.version.Load()
}

// ReadFullConfig returns the current effective config (YAML merged with DB overrides).
// Implements ConfigWatcher.
func (c *ConfigAPI) ReadFullConfig() (Config, error) {
	return c.readConfig()
}

// writeConfig writes config to YAML file AND to DB (if store is configured),
// then bumps the version counter so watchers detect the change.
func (c *ConfigAPI) writeConfig(cfg *Config) error {
	// Write to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.configPath, data, 0o644); err != nil {
		return err
	}

	// Write to DB
	if c.store != nil {
		if err := c.writeConfigToDB(cfg); err != nil {
			log.Printf("[config] write to DB warning: %v", err)
			// Don't fail - YAML write succeeded
		}
	}

	// Signal change to all watchers.
	c.version.Add(1)
	return nil
}

// writeConfigToDB persists config sections to the DB store.
func (c *ConfigAPI) writeConfigToDB(cfg *Config) error {
	agentsData, err := json.Marshal(map[string]any{
		"supervisor": cfg.Supervisor,
		"sub_agents": cfg.SubAgents,
	})
	if err != nil {
		return err
	}
	if err := c.store.UpsertConfigSetting("agents", string(agentsData)); err != nil {
		return err
	}

	memData, err := json.Marshal(cfg.Memory)
	if err != nil {
		return err
	}
	if err := c.store.UpsertConfigSetting("memory", string(memData)); err != nil {
		return err
	}

	sbData, err := json.Marshal(cfg.Sandbox)
	if err != nil {
		return err
	}
	return c.store.UpsertConfigSetting("sandbox", string(sbData))
}

func writeConfigJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
