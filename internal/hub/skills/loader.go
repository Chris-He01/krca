package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFileNames lists candidate skill file names in priority order.
// The first match in a skill directory is used.
var skillFileNames = []string{
	"SKILL.md",
	"SKILL.yaml",
	"SKILL.yml",
	"SKILL.txt",
	"SKILL.sh",
	"SKILL.py",
	"SKILL.json",
}

var errInvalidSkillYAML = errors.New("invalid skill YAML frontmatter")

// Skill represents a loaded skill with metadata and content.
type Skill struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Keywords    []string `json:"keywords" yaml:"keywords"`
	Scope       string   `json:"scope" yaml:"scope"`
	Always      bool     `json:"always" yaml:"always"`
	Content     string   `json:"content" yaml:"-"`
	Path        string   `json:"path" yaml:"-"`
}

// Loader loads and manages skills from the filesystem.
type Loader struct {
	skillDir  string
	extraDirs []string // additional dirs loaded in order (for NewMultiLoader)
	skills    []Skill
}

func NewLoader(skillDir string) *Loader {
	return &Loader{skillDir: skillDir}
}

// NewMultiLoader returns a Loader that reads from multiple directories in order.
// Later directories can override skills with the same name from earlier ones.
func NewMultiLoader(dirs ...string) *Loader {
	return &Loader{extraDirs: dirs}
}

// Load reads all skills from the configured directories.
func (l *Loader) Load() error {
	l.skills = nil

	dirs := l.extraDirs
	if l.skillDir != "" {
		dirs = append(dirs, l.skillDir)
	}
	for _, dir := range dirs {
		if err := l.loadFromDir(dir); err != nil {
			return err
		}
	}

	sort.Slice(l.skills, func(i, j int) bool {
		return l.skills[i].Name < l.skills[j].Name
	})

	return nil
}

// loadFromDir loads all skills from a single root directory (walks scope subdirs).
func (l *Loader) loadFromDir(skillDir string) error {
	if skillDir == "" {
		return nil
	}
	info, err := os.Stat(skillDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat skills dir %q: %w", skillDir, err)
	}
	if !info.IsDir() {
		return nil
	}

	scopes, err := os.ReadDir(skillDir)
	if err != nil {
		return fmt.Errorf("read skills dir %q: %w", skillDir, err)
	}

	for _, scopeEntry := range scopes {
		if !scopeEntry.IsDir() {
			continue
		}
		scopeName := scopeEntry.Name()
		scopeDir := filepath.Join(skillDir, scopeName)

		if scopeName == "user" {
			// user/ scope: one extra nesting level → user/{username}/{skill_name}/SKILL.*
			userDirs, readErr := os.ReadDir(scopeDir)
			if readErr != nil {
				continue
			}
			for _, userEntry := range userDirs {
				if !userEntry.IsDir() {
					continue
				}
				l.loadSkillsInDir(filepath.Join(scopeDir, userEntry.Name()), "user/"+userEntry.Name())
			}
		} else if scopeName == "scene" {
			// scene/ scope: one extra nesting level → scene/{sceneID}/{skill_name}/SKILL.*
			sceneDirs, readErr := os.ReadDir(scopeDir)
			if readErr != nil {
				continue
			}
			for _, sceneEntry := range sceneDirs {
				if !sceneEntry.IsDir() {
					continue
				}
				l.loadSkillsInDir(filepath.Join(scopeDir, sceneEntry.Name()), "scene/"+sceneEntry.Name())
			}
		} else {
			l.loadSkillsInDir(scopeDir, scopeName)
		}
	}
	return nil
}

// loadSkillsInDir loads all skills from subdirectories of dir, assigning the given scope.
func (l *Loader) loadSkillsInDir(dir, scope string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := findSkillFile(filepath.Join(dir, entry.Name()))
		if skillPath == "" {
			continue
		}
		skill, skip, parseErr := parseSkillFile(skillPath, scope)
		if parseErr != nil {
			log.Printf("[skills] error loading %s: %v", skillPath, parseErr)
			continue
		}
		if skip {
			continue
		}
		l.skills = append(l.skills, skill)
	}
}

// ListSkills returns all loaded skills.
func (l *Loader) ListSkills() []Skill {
	return l.skills
}

// ListSkillsByScope returns skills matching the given scope.
func (l *Loader) ListSkillsByScope(scope string) []Skill {
	var result []Skill
	for _, s := range l.skills {
		if s.Scope == scope {
			result = append(result, s)
		}
	}
	return result
}

// GetSkill returns a skill by name.
func (l *Loader) GetSkill(name string) (Skill, bool) {
	for _, s := range l.skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

// GetAlwaysSkills returns the combined content of skills with always=true.
func (l *Loader) GetAlwaysSkills() string {
	var sb strings.Builder
	for _, s := range l.skills {
		if s.Always {
			sb.WriteString(fmt.Sprintf("## %s\n%s\n\n", s.Name, s.Content))
		}
	}
	return sb.String()
}

// GetAlwaysSkillsForUser returns always-on skills visible to the given user.
// Shared scopes + user's own scope.
func (l *Loader) GetAlwaysSkillsForUser(userID string) string {
	var sb strings.Builder
	for _, s := range l.skills {
		if !s.Always {
			continue
		}
		if !isVisibleToUser(s.Scope, userID) {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s\n%s\n\n", s.Name, s.Content))
	}
	return sb.String()
}

// BuildSummaryForUser returns skill summary filtered by user visibility.
func (l *Loader) BuildSummaryForUser(userID string) string {
	if len(l.skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<skills note=\"NOT callable tools — use get_skill(name) to load instructions, then exec_shell to execute\">\n")
	for _, s := range l.skills {
		if s.Always {
			continue
		}
		if !isVisibleToUser(s.Scope, userID) {
			continue
		}
		sb.WriteString(fmt.Sprintf("  <skill name=%q scope=%q", s.Name, s.Scope))
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf(" description=%q", s.Description))
		}
		if len(s.Keywords) > 0 {
			sb.WriteString(fmt.Sprintf(" keywords=%q", strings.Join(s.Keywords, ",")))
		}
		sb.WriteString(" />\n")
	}
	sb.WriteString("</skills>\n")
	return sb.String()
}

// isVisibleToUser returns true if the scope is visible to the given user.
// "user/xxx" scopes are private; "scene/xxx" scopes are excluded (use isVisibleToScene).
func isVisibleToUser(scope, userID string) bool {
	if strings.HasPrefix(scope, "scene/") {
		return false // scene scopes not visible in user context
	}
	if strings.HasPrefix(scope, "user/") {
		owner := strings.TrimPrefix(scope, "user/")
		return owner == userID
	}
	return true // shared scope
}

// isVisibleToScene returns true if the scope is visible in the given scene.
// "scene/xxx" scopes only match the specified sceneID; "user/xxx" scopes are excluded.
// All other scopes (shared/_common) are always visible.
func isVisibleToScene(scope, sceneID string) bool {
	if strings.HasPrefix(scope, "user/") {
		return false // user scopes not visible in scene context
	}
	if strings.HasPrefix(scope, "scene/") {
		return scope == "scene/"+sceneID
	}
	return true // shared scope
}

// GetAlwaysSkillsForScene returns always-on skills visible in the given scene.
func (l *Loader) GetAlwaysSkillsForScene(sceneID string) string {
	var sb strings.Builder
	for _, s := range l.skills {
		if !s.Always {
			continue
		}
		if !isVisibleToScene(s.Scope, sceneID) {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s\n%s\n\n", s.Name, s.Content))
	}
	return sb.String()
}

// BuildSummaryForScene returns skill summary filtered by scene visibility.
func (l *Loader) BuildSummaryForScene(sceneID string) string {
	if len(l.skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<skills note=\"NOT callable tools — use get_skill(name) to load instructions, then exec_shell to execute\">\n")
	for _, s := range l.skills {
		if s.Always {
			continue
		}
		if !isVisibleToScene(s.Scope, sceneID) {
			continue
		}
		sb.WriteString(fmt.Sprintf("  <skill name=%q scope=%q", s.Name, s.Scope))
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf(" description=%q", s.Description))
		}
		if len(s.Keywords) > 0 {
			sb.WriteString(fmt.Sprintf(" keywords=%q", strings.Join(s.Keywords, ",")))
		}
		sb.WriteString(" />\n")
	}
	sb.WriteString("</skills>\n")
	return sb.String()
}

// BuildSummary returns an XML-formatted summary of available skills.
func (l *Loader) BuildSummary() string {
	if len(l.skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<skills note=\"NOT callable tools — use get_skill(name) to load instructions, then exec_shell to execute\">\n")
	for _, s := range l.skills {
		if s.Always {
			continue
		}
		sb.WriteString(fmt.Sprintf("  <skill name=%q scope=%q", s.Name, s.Scope))
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf(" description=%q", s.Description))
		}
		if len(s.Keywords) > 0 {
			sb.WriteString(fmt.Sprintf(" keywords=%q", strings.Join(s.Keywords, ",")))
		}
		sb.WriteString(" />\n")
	}
	sb.WriteString("</skills>\n")
	return sb.String()
}

// GetScopes returns all unique scope names.
func (l *Loader) GetScopes() []string {
	seen := make(map[string]bool)
	for _, s := range l.skills {
		seen[s.Scope] = true
	}
	scopes := make([]string, 0, len(seen))
	for s := range seen {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)
	return scopes
}

// findSkillFile returns the path of the first matching skill file in dir,
// or empty string if none found.
func findSkillFile(dir string) string {
	for _, name := range skillFileNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func parseSkillFile(path, scope string) (Skill, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Skill{}, true, nil
		}
		return Skill{}, false, fmt.Errorf("read skill %q: %w", path, err)
	}

	meta, body, err := parseFrontmatter(content)
	if err != nil {
		if errors.Is(err, errInvalidSkillYAML) {
			return Skill{}, true, nil
		}
		return Skill{}, false, fmt.Errorf("parse skill %q: %w", path, err)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return Skill{}, true, nil
	}

	return Skill{
		Name:        strings.TrimSpace(meta.Name),
		Description: strings.TrimSpace(meta.Description),
		Keywords:    sanitizeKeywords(meta.Keywords),
		Scope:       scope,
		Always:      meta.Always,
		Content:     strings.TrimSpace(body),
		Path:        path,
	}, false, nil
}

type skillFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Keywords    []string `yaml:"keywords"`
	Always      bool     `yaml:"always"`
}

func parseFrontmatter(content []byte) (skillFrontmatter, string, error) {
	text := strings.TrimPrefix(string(content), "\uFEFF")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return skillFrontmatter{}, "", errors.New("missing YAML frontmatter")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return skillFrontmatter{}, "", errors.New("missing closing frontmatter separator")
	}

	frontmatter := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")

	var meta skillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return skillFrontmatter{}, "", fmt.Errorf("%w: %v", errInvalidSkillYAML, err)
	}

	return meta, body, nil
}

func sanitizeKeywords(keywords []string) []string {
	if len(keywords) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keywords))
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		normalized := strings.ToLower(strings.TrimSpace(keyword))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
