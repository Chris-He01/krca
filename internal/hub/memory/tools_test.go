package memory

import (
	"context"
	"strings"
	"testing"
)

func TestUpdateMemoryTool(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir, 7)
	tool := NewUpdateMemoryTool(store)

	ctx := context.Background()

	// Test writing memory
	result, err := tool.InvokableRun(ctx, `{"content": "# Important\nTest pattern"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.HasPrefix(result, "ok:") {
		t.Errorf("expected ok prefix, got %q", result)
	}

	// Verify it was written
	content, err := store.ReadLongTerm()
	if err != nil {
		t.Fatalf("ReadLongTerm: %v", err)
	}
	if content != "# Important\nTest pattern" {
		t.Errorf("unexpected content: %q", content)
	}

	// Test empty content
	result, err = tool.InvokableRun(ctx, `{"content": ""}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.HasPrefix(result, "error:") {
		t.Errorf("expected error for empty content, got %q", result)
	}
}

func TestAppendJournalTool(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir, 7)
	tool := NewAppendJournalTool(store)

	ctx := context.Background()

	result, err := tool.InvokableRun(ctx, `{"content": "Found high CPU usage on node-24"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.HasPrefix(result, "ok: journal entry appended") {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify it was written with timestamp
	today, err := store.ReadToday()
	if err != nil {
		t.Fatalf("ReadToday: %v", err)
	}
	if !strings.Contains(today, "Found high CPU usage on node-24") {
		t.Errorf("journal entry not found in today's file: %q", today)
	}
	// Check timestamp format [HH:MM:SS]
	if !strings.Contains(today, "[") || !strings.Contains(today, "]") {
		t.Errorf("expected timestamp in journal entry: %q", today)
	}
}

func TestReadMemoryTool(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMemoryStore(tmpDir, 7)
	tool := NewReadMemoryTool(store)

	ctx := context.Background()

	// Empty memory
	result, err := tool.InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(result, "empty") {
		t.Errorf("expected empty message, got %q", result)
	}

	// Write some memory then read
	_ = store.WriteLongTerm("# Test Memory\nSome important info")
	result, err = tool.InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(result, "Test Memory") {
		t.Errorf("expected memory content, got %q", result)
	}
}
