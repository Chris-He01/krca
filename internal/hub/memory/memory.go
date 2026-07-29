package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MemoryStore manages long-term memory and daily journal files.
type MemoryStore struct {
	workspace  string
	recentDays int
	memDirPath string // override for memoryDir(); empty means default

	// onWrite is called after every successful file write with (relPath, content).
	// Used for async backends (e.g. Redis sync). May be nil.
	onWrite func(relPath, content string)
}

// SetWriteHook registers a callback invoked after each successful write.
func (m *MemoryStore) SetWriteHook(fn func(relPath, content string)) {
	m.onWrite = fn
}

func NewMemoryStore(workspace string, recentDays int) *MemoryStore {
	if recentDays <= 0 {
		recentDays = 7
	}
	return &MemoryStore{workspace: workspace, recentDays: recentDays}
}

// ForUser returns a new MemoryStore scoped to a specific user's subdirectory.
// The user store reads/writes under {workspace}/memory/user/{userID}/ instead
// of {workspace}/memory/.
func (m *MemoryStore) ForUser(userID string) *MemoryStore {
	prefix := "user/" + userID + "/"
	var wrappedOnWrite func(relPath, content string)
	if m.onWrite != nil {
		wrappedOnWrite = func(relPath, content string) {
			m.onWrite(prefix+relPath, content)
		}
	}
	return &MemoryStore{
		workspace:  m.workspace,
		recentDays: m.recentDays,
		memDirPath: filepath.Join(m.workspace, "memory", "user", userID),
		onWrite:    wrappedOnWrite,
	}
}

// ForScene returns a new MemoryStore scoped to a specific scene's subdirectory.
// The scene store reads/writes under {workspace}/memory/scene/{sceneID}/.
func (m *MemoryStore) ForScene(sceneID string) *MemoryStore {
	prefix := "scene/" + sceneID + "/"
	var wrappedOnWrite func(relPath, content string)
	if m.onWrite != nil {
		wrappedOnWrite = func(relPath, content string) {
			m.onWrite(prefix+relPath, content)
		}
	}
	return &MemoryStore{
		workspace:  m.workspace,
		recentDays: m.recentDays,
		memDirPath: filepath.Join(m.workspace, "memory", "scene", sceneID),
		onWrite:    wrappedOnWrite,
	}
}

// Workspace returns the workspace path (used for file tree roots).
func (m *MemoryStore) Workspace() string {
	return m.workspace
}

func (m *MemoryStore) memoryDir() string {
	if m.memDirPath != "" {
		return m.memDirPath
	}
	return filepath.Join(m.workspace, "memory")
}

func (m *MemoryStore) ensureDir() error {
	return os.MkdirAll(m.memoryDir(), 0755)
}

// ReadLongTerm reads the persistent MEMORY.md file.
func (m *MemoryStore) ReadLongTerm() (string, error) {
	data, err := os.ReadFile(filepath.Join(m.memoryDir(), "MEMORY.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// WriteLongTerm overwrites the persistent MEMORY.md file.
func (m *MemoryStore) WriteLongTerm(content string) error {
	if err := m.ensureDir(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(m.memoryDir(), "MEMORY.md"), []byte(content), 0644); err != nil {
		return err
	}
	if m.onWrite != nil {
		m.onWrite("MEMORY.md", content)
	}
	return nil
}

func (m *MemoryStore) todayFile() string {
	return filepath.Join(m.memoryDir(), time.Now().Format("2006-01-02")+".md")
}

// ReadToday reads today's journal file.
func (m *MemoryStore) ReadToday() (string, error) {
	data, err := os.ReadFile(m.todayFile())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// AppendToday appends a line to today's journal file.
func (m *MemoryStore) AppendToday(content string) error {
	if err := m.ensureDir(); err != nil {
		return err
	}
	f, err := os.OpenFile(m.todayFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.WriteString(content + "\n"); err != nil {
		return err
	}
	if m.onWrite != nil {
		// Read back the full file to sync to Redis
		if full, readErr := os.ReadFile(m.todayFile()); readErr == nil {
			relName := filepath.Base(m.todayFile())
			m.onWrite(relName, string(full))
		}
	}
	return nil
}

// GetRecentMemories reads the last N days of journal entries.
func (m *MemoryStore) GetRecentMemories(days int) (string, error) {
	dir := m.memoryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var dateFiles []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".md") && name != "MEMORY.md" {
			dateFiles = append(dateFiles, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dateFiles)))

	if days > 0 && len(dateFiles) > days {
		dateFiles = dateFiles[:days]
	}

	var sb strings.Builder
	for _, name := range dateFiles {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		date := strings.TrimSuffix(name, ".md")
		sb.WriteString(fmt.Sprintf("## %s\n%s\n\n", date, content))
	}
	return sb.String(), nil
}

// GetMemoryContext assembles long-term + recent memory for LLM system prompt.
func (m *MemoryStore) GetMemoryContext() string {
	var sb strings.Builder

	longTerm, err := m.ReadLongTerm()
	if err == nil && strings.TrimSpace(longTerm) != "" {
		sb.WriteString("## Long-term Memory\n")
		sb.WriteString(longTerm)
		sb.WriteString("\n\n")
	}

	recent, err := m.GetRecentMemories(m.recentDays)
	if err == nil && strings.TrimSpace(recent) != "" {
		sb.WriteString("## Recent Journal\n")
		sb.WriteString(recent)
		sb.WriteString("\n")
	}

	return sb.String()
}
