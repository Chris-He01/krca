package skills

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"knsight-go/internal/hub/user"
)

// Handler provides HTTP handlers for Skills CRUD.
type Handler struct {
	loader  *Loader
	onWrite func(scope, name, relPath, content string) error // optional hook for Redis sync; error causes 500
}

func NewHandler(loader *Loader) *Handler {
	return &Handler{loader: loader}
}

// SetWriteHook registers a callback invoked after each successful skill file write.
// If the hook returns an error, the PUT request fails with 500.
func (h *Handler) SetWriteHook(fn func(scope, name, relPath, content string) error) {
	h.onWrite = fn
}

// RegisterRoutes mounts skill routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/skills", h.handleList)
	mux.HandleFunc("/v1/skills/scopes", h.handleScopes)
	mux.HandleFunc("/v1/skills/", h.handleSkill)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	u := user.FromContext(r.Context())
	skills := h.loader.ListSkills()

	// Filter by query params
	scope := r.URL.Query().Get("scope")
	name := r.URL.Query().Get("name")

	var filtered []Skill
	for _, s := range skills {
		// user/{xxx} scoped skills are only visible to that user
		if !isSkillVisibleToUser(s.Scope, u.ID) {
			continue
		}
		if scope != "" && s.Scope != scope {
			continue
		}
		if name != "" && !strings.Contains(strings.ToLower(s.Name), strings.ToLower(name)) {
			continue
		}
		filtered = append(filtered, s)
	}

	writeJSON(w, filtered)
}

// isSkillVisibleToUser returns true if the skill scope is visible to the given user.
// Scopes starting with "user/" are private — only the owning user can see them.
// All other scopes (_common, service, host, etc.) are shared.
func isSkillVisibleToUser(scope, userID string) bool {
	if !strings.HasPrefix(scope, "user/") {
		return true // shared scope
	}
	// Extract username from scope "user/{username}"
	owner := strings.TrimPrefix(scope, "user/")
	return owner == userID
}

func (h *Handler) handleScopes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	u := user.FromContext(r.Context())
	allScopes := h.loader.GetScopes()
	var visible []string
	for _, s := range allScopes {
		if isSkillVisibleToUser(s, u.ID) {
			visible = append(visible, s)
		}
	}
	writeJSON(w, visible)
}

func (h *Handler) handleSkill(w http.ResponseWriter, r *http.Request) {
	// Path: /v1/skills/{scope}/{name} or /v1/skills/user/{username}/{name}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/skills/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "path must be /v1/skills/{scope}/{name}", http.StatusBadRequest)
		return
	}

	var scope, name string
	if parts[0] == "user" && len(parts) >= 3 && parts[2] != "" {
		// /v1/skills/user/{username}/{name}
		scope = "user/" + parts[1]
		name = parts[2]
	} else {
		scope = parts[0]
		name = parts[1]
	}

	// Check user access for user-scoped skills
	u := user.FromContext(r.Context())
	if !isSkillVisibleToUser(scope, u.ID) {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		skill, ok := h.loader.GetSkill(name)
		if !ok || skill.Scope != scope {
			http.Error(w, "skill not found", http.StatusNotFound)
			return
		}
		writeJSON(w, skill)

	case http.MethodPut:
		var body struct {
			Content     string   `json:"content"`
			Description string   `json:"description"`
			Keywords    []string `json:"keywords"`
			Always      bool     `json:"always"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// For now, write to disk and reload
		if err := writeSkillFile(h.loader.skillDir, scope, name, body.Description, body.Keywords, body.Always, body.Content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := h.loader.Load(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if h.onWrite != nil {
			relPath := filepath.ToSlash(filepath.Join(scope, name, "SKILL.md"))
			if err := h.onWrite(scope, name, relPath, body.Content); err != nil {
				http.Error(w, "redis write failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		skill, _ := h.loader.GetSkill(name)
		writeJSON(w, skill)

	case http.MethodDelete:
		if err := deleteSkillFile(h.loader.skillDir, scope, name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := h.loader.Load(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeSkillFile(skillDir, scope, name, description string, keywords []string, always bool, content string) error {
	dir := filepath.Join(skillDir, scope, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	if description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", description))
	}
	if len(keywords) > 0 {
		sb.WriteString("keywords:\n")
		for _, kw := range keywords {
			sb.WriteString(fmt.Sprintf("  - %s\n", kw))
		}
	}
	if always {
		sb.WriteString("always: true\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(content)

	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(sb.String()), 0644)
}

func deleteSkillFile(skillDir, scope, name string) error {
	dir := filepath.Join(skillDir, scope, name)
	return os.RemoveAll(dir)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
