package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWorkspaceDirAndCleanup(t *testing.T) {
	baseDir := t.TempDir()
	sb, err := New(Config{
		WorkspaceDir:     filepath.Join(baseDir, "workspace"),
		SessionBaseDir:   filepath.Join(baseDir, "sessions"),
		SessionMaxAgeSec: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runDir := sb.RunWorkspaceDir("run-1", "user-1")
	if filepath.Base(runDir) != "session-run-1" {
		t.Fatalf("unexpected run workspace: %s", runDir)
	}
	old := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(runDir, old, old); err != nil {
		t.Fatal(err)
	}
	sb.CleanExpiredRunWorkspaces()
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("expired run workspace still exists: %v", err)
	}
}

func TestSandboxExecShell(t *testing.T) {
	sb, err := New(Config{
		MaxOutputBytes:    10_000,
		CommandTimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	output, err := sb.ExecShell(context.Background(), "echo hello", "")
	if err != nil {
		t.Fatalf("ExecShell: %v", err)
	}
	if output != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", output)
	}
}

func TestSandboxExecShellDenied(t *testing.T) {
	sb, err := New(Config{
		MaxOutputBytes:    10_000,
		CommandTimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	output, err := sb.ExecShell(context.Background(), "rm -rf /", "")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "error:") {
		t.Fatalf("expected error string in output, got %q", output)
	}
}

func TestSandboxReadWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	sb, err := New(Config{
		WorkspaceDir:   tmpDir,
		MaxOutputBytes: 10_000,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()

	msg, err := sb.WriteFile(ctx, "test.txt", "hello world", tmpDir)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if msg == "" {
		t.Fatal("expected non-empty write result")
	}

	content, err := sb.ReadFile(ctx, "test.txt", tmpDir)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestSandboxListDir(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644)
	_ = os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	sb, err := New(Config{
		WorkspaceDir:   tmpDir,
		MaxOutputBytes: 10_000,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	output, err := sb.ListDir(context.Background(), ".", tmpDir)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty listing")
	}
}

func TestSandboxLegacyWorkspacePrefixUsesCurrentUserWorkspace(t *testing.T) {
	sb, err := New(Config{
		WorkspaceDir:   "sandbox/workspace",
		MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]string{
		"sandbox/workspace":              ".",
		"sandbox/workspace/metrics.json": "metrics.json",
		"metrics.json":                   "metrics.json",
	}
	for input, want := range tests {
		if got := sb.normalizeWorkspacePath(input); got != want {
			t.Errorf("normalizeWorkspacePath(%q) = %q, want %q", input, got, want)
		}
	}
}
