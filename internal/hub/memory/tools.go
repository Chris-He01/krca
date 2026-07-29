package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"knsight-go/internal/hub/user"
)

// resolveStore returns the appropriate store based on scope parameter and user context.
// scope="shared" → shared store (team knowledge)
// scope="user" or scope="" (default) → user-private store (personal summaries)
// Falls back to shared if user is visitor.
func resolveStore(baseStore *MemoryStore, scope string, ctx context.Context) (*MemoryStore, string) {
	if scope == "shared" {
		return baseStore, "shared"
	}
	// Default to user-scoped for personal data
	u := user.FromContext(ctx)
	if u.IsVisitor() {
		return baseStore, "shared"
	}
	return baseStore.ForUser(u.ID), "user/" + u.ID
}

// --- UpdateMemoryTool ---

// UpdateMemoryTool allows the LLM to overwrite long-term memory (MEMORY.md).
type UpdateMemoryTool struct {
	store *MemoryStore
}

func NewUpdateMemoryTool(store *MemoryStore) *UpdateMemoryTool {
	return &UpdateMemoryTool{store: store}
}

func (t *UpdateMemoryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "update_memory",
		Desc: `Update long-term memory (MEMORY.md). This overwrites the entire file.
Default scope is "user" (personal): diagnostic summaries, user-specific findings, personal notes.
Use scope="shared" ONLY for team-wide knowledge: reusable diagnostic patterns, domain facts, common procedures that benefit ALL users.
Always read_memory first to avoid losing existing content.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"content": {Type: schema.String, Desc: "Full content to write to MEMORY.md", Required: true},
			"scope":   {Type: schema.String, Desc: `"user" (default) for personal memory, "shared" for team-wide knowledge`, Required: false},
		}),
	}, nil
}

func (t *UpdateMemoryTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Content string `json:"content"`
		Scope   string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Content == "" {
		return "error: content is required", nil
	}
	store, scopeLabel := resolveStore(t.store, args.Scope, ctx)
	if err := store.WriteLongTerm(args.Content); err != nil {
		return fmt.Sprintf("error: failed to write memory: %s", err.Error()), nil
	}
	return fmt.Sprintf("ok: long-term memory updated in scope=%s (%d bytes)", scopeLabel, len(args.Content)), nil
}

// --- AppendJournalTool ---

// AppendJournalTool allows the LLM to append entries to today's journal.
type AppendJournalTool struct {
	store *MemoryStore
}

func NewAppendJournalTool(store *MemoryStore) *AppendJournalTool {
	return &AppendJournalTool{store: store}
}

func (t *AppendJournalTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "append_journal",
		Desc: `Append an entry to today's journal. Each entry is timestamped automatically.
Default scope is "user" (personal): each user's diagnostic findings, task progress, personal notes.
Use scope="shared" ONLY for team-wide observations: common incidents, shared learnings that benefit ALL users.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"content": {Type: schema.String, Desc: "Text to append to today's journal", Required: true},
			"scope":   {Type: schema.String, Desc: `"user" (default) for personal journal, "shared" for team-wide journal`, Required: false},
		}),
	}, nil
}

func (t *AppendJournalTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Content string `json:"content"`
		Scope   string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Content == "" {
		return "error: content is required", nil
	}
	store, scopeLabel := resolveStore(t.store, args.Scope, ctx)
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), args.Content)
	if err := store.AppendToday(entry); err != nil {
		return fmt.Sprintf("error: failed to append journal: %s", err.Error()), nil
	}
	return fmt.Sprintf("ok: journal entry appended to scope=%s", scopeLabel), nil
}

// --- ReadMemoryTool ---

// ReadMemoryTool allows the LLM to read current memory context on demand.
type ReadMemoryTool struct {
	store *MemoryStore
}

func NewReadMemoryTool(store *MemoryStore) *ReadMemoryTool {
	return &ReadMemoryTool{store: store}
}

func (t *ReadMemoryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_memory",
		Desc: `Read memory context. Default scope is "all" which reads both shared and personal memory.
Use scope="shared" for team memory only, scope="user" for personal memory only.
Always read before updating to avoid losing existing content.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"scope": {Type: schema.String, Desc: `"all" (default) for both, "shared" for team memory, "user" for personal memory`, Required: false},
		}),
	}, nil
}

func (t *ReadMemoryTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Scope string `json:"scope"`
	}
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}

	switch args.Scope {
	case "user":
		u := user.FromContext(ctx)
		if u.IsVisitor() {
			return "No user memory available for visitor.", nil
		}
		userCtx := t.store.ForUser(u.ID).GetMemoryContext()
		if userCtx == "" {
			return fmt.Sprintf("User memory for %s is empty.", u.ID), nil
		}
		return fmt.Sprintf("## User Memory (%s)\n%s", u.ID, userCtx), nil

	case "shared":
		sharedCtx := t.store.GetMemoryContext()
		if sharedCtx == "" {
			return "Shared memory is empty. No long-term memory or recent journal entries found.", nil
		}
		return sharedCtx, nil

	default: // "all" or empty — read both shared + user
		var result string
		sharedCtx := t.store.GetMemoryContext()
		if sharedCtx != "" {
			result += "## Shared Memory\n" + sharedCtx + "\n"
		}
		u := user.FromContext(ctx)
		if !u.IsVisitor() {
			userCtx := t.store.ForUser(u.ID).GetMemoryContext()
			if userCtx != "" {
				result += fmt.Sprintf("## User Memory (%s)\n%s\n", u.ID, userCtx)
			}
		}
		if result == "" {
			return "Memory is empty. No shared or user memory found.", nil
		}
		return result, nil
	}
}
