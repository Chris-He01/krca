package sandbox

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	WorkspaceDir        string
	DataDir             string // root data dir (for skills/memory fallback reads)
	DenyPatterns        []string
	MaxOutputBytes      int
	CommandTimeoutSec   int
	RestrictToWorkspace bool
	SessionBaseDir      string
	SessionMaxAgeSec    int
}

type Sandbox struct {
	guard    *Guard
	executor Executor
	cfg      Config
}

func New(cfg Config) (*Sandbox, error) {
	guard, err := NewGuard(cfg.DenyPatterns, cfg.RestrictToWorkspace)
	if err != nil {
		return nil, err
	}
	executor := NewDirectExecutor(cfg.CommandTimeoutSec, cfg.MaxOutputBytes)
	return &Sandbox{
		guard:    guard,
		executor: executor,
		cfg:      cfg,
	}, nil
}

// UserWorkspaceDir returns the per-user workspace directory without modifying shared state.
// If userID is empty or "visitor", returns the shared workspace.
func (s *Sandbox) UserWorkspaceDir(userID string) string {
	if userID == "" || userID == "visitor" {
		return s.cfg.WorkspaceDir
	}
	userDir := filepath.Join(s.cfg.WorkspaceDir, "user", userID)
	_ = os.MkdirAll(userDir, 0755)
	return userDir
}

// RunWorkspaceDir returns the temporary workspace shared by all agents in one
// diagnosis run. Run workspaces are TTL-managed under SessionBaseDir.
func (s *Sandbox) RunWorkspaceDir(runID, userID string) string {
	if runID == "" {
		return s.UserWorkspaceDir(userID)
	}
	dir, err := EnsureSessionDir(s.cfg.SessionBaseDir, filepath.Base(runID))
	if err != nil {
		return s.UserWorkspaceDir(userID)
	}
	return dir
}

// CleanExpiredRunWorkspaces removes run-scoped files older than the configured TTL.
func (s *Sandbox) CleanExpiredRunWorkspaces() {
	if s.cfg.SessionBaseDir == "" || s.cfg.SessionMaxAgeSec <= 0 {
		return
	}
	_, _ = CleanExpiredSessions(
		s.cfg.SessionBaseDir,
		time.Duration(s.cfg.SessionMaxAgeSec)*time.Second,
	)
}

// WorkspaceDir returns the current (default) workspace directory.
func (s *Sandbox) WorkspaceDir() string {
	return s.cfg.WorkspaceDir
}

// ExecShell executes a shell command in the given workDir.
func (s *Sandbox) ExecShell(ctx context.Context, command, workDir string) (string, error) {
	if err := s.guard.CheckCommand(command); err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	if workDir == "" {
		workDir = s.cfg.WorkspaceDir
	}
	output, err := s.executor.Execute(ctx, command, workDir)
	if err != nil {
		return fmt.Sprintf("%s\nerror: %s", output, err.Error()), nil
	}
	return output, nil
}

// ReadFile reads a file. Errors (not found, permission denied, etc.) are
// returned as string output so the LLM can handle them gracefully.
func (s *Sandbox) ReadFile(_ context.Context, path, workDir string) (string, error) {
	if err := s.guard.CheckPath(path); err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	fullPath := s.resolvePathIn(path, workDir)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	content := string(data)
	if len(content) > s.cfg.MaxOutputBytes {
		content = content[:s.cfg.MaxOutputBytes] + fmt.Sprintf("\n... (truncated at %d bytes)", s.cfg.MaxOutputBytes)
	}
	return content, nil
}

// WriteFile writes a file. Errors are returned as string output.
func (s *Sandbox) WriteFile(_ context.Context, path, content, workDir string) (string, error) {
	if err := s.guard.CheckPath(path); err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	fullPath := s.resolvePathIn(path, workDir)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("error: create directory: %s", err.Error()), nil
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Sprintf("error: write file: %s", err.Error()), nil
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

// ListDir lists a directory. Errors are returned as string output.
func (s *Sandbox) ListDir(_ context.Context, path, workDir string) (string, error) {
	if err := s.guard.CheckPath(path); err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	fullPath := s.resolvePathIn(path, workDir)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	var sb strings.Builder
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		prefix := "-"
		if e.IsDir() {
			prefix = "d"
		}
		sb.WriteString(fmt.Sprintf("%s %8d %s %s\n", prefix, info.Size(), info.ModTime().Format("2006-01-02 15:04"), e.Name()))
	}
	return sb.String(), nil
}

// ReadImageBase64 reads an image file and returns it as base64 with MIME type.
func (s *Sandbox) ReadImageBase64(_ context.Context, path, workDir string) (string, string, error) {
	if err := s.guard.CheckPath(path); err != nil {
		return "", "", fmt.Errorf("%s", err.Error())
	}
	fullPath := s.resolvePathIn(path, workDir)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", "", err
	}
	// Determine MIME type from extension
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := "image/png"
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	}
	return base64.StdEncoding.EncodeToString(data), mimeType, nil
}

func (s *Sandbox) resolvePathIn(path, workDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	path = s.normalizeWorkspacePath(path)
	if workDir != "" && s.cfg.SessionBaseDir != "" {
		rel, err := filepath.Rel(filepath.Clean(s.cfg.SessionBaseDir), filepath.Clean(workDir))
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.Join(workDir, path)
		}
	}
	// Try user workspace first
	if workDir != "" {
		candidate := filepath.Join(workDir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback: try shared workspace
	if s.cfg.WorkspaceDir != "" {
		candidate := filepath.Join(s.cfg.WorkspaceDir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback: try data dir (for skills/memory scripts)
	if s.cfg.DataDir != "" {
		candidate := filepath.Join(s.cfg.DataDir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Default: resolve relative to user workspace (even if not exists, for write operations)
	if workDir != "" {
		return filepath.Join(workDir, path)
	}
	if s.cfg.WorkspaceDir != "" {
		return filepath.Join(s.cfg.WorkspaceDir, path)
	}
	return path
}

// normalizeWorkspacePath keeps compatibility with older prompts that told
// agents to prefix paths with the configured workspace directory. Tools already
// run relative to the current user's workspace, so retaining that prefix would
// duplicate the path.
func (s *Sandbox) normalizeWorkspacePath(path string) string {
	if s.cfg.WorkspaceDir == "" {
		return path
	}
	cleanPath := filepath.Clean(path)
	workspace := filepath.Clean(s.cfg.WorkspaceDir)
	if cleanPath == workspace {
		return "."
	}
	prefix := workspace + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return strings.TrimPrefix(cleanPath, prefix)
	}
	return path
}
