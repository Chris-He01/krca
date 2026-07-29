package hub

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// ErrorNotifier sends error alerts to external channels (e.g. Kim group chat).
type ErrorNotifier struct {
	mu       sync.Mutex
	sender   func(markdown string) error // kim.Client.SendMarkdown
	cooldown map[string]time.Time        // dedup: error key → last sent time
}

var globalNotifier *ErrorNotifier

// SetErrorNotifier configures the global error notifier.
func SetErrorNotifier(sender func(markdown string) error) {
	globalNotifier = &ErrorNotifier{
		sender:   sender,
		cooldown: make(map[string]time.Time),
	}
}

// NotifyError sends an error alert if the notifier is configured.
// Deduplicates by errKey (e.g. "mcp:CloudStability") with a 5-minute cooldown.
// Errors that originate from a user/client cancellation (context canceled,
// closed SSE, etc.) are dropped without alerting — those are not bugs, they
// reflect the user pressing Stop or the browser tab closing.
func NotifyError(errKey string, params ErrorParams) {
	if globalNotifier == nil || globalNotifier.sender == nil {
		return
	}
	if IsCancellationError(params.Error) {
		log.Printf("[kim] suppress alert %s: user/client cancellation", errKey)
		return
	}
	globalNotifier.mu.Lock()
	if last, ok := globalNotifier.cooldown[errKey]; ok && time.Since(last) < 5*time.Minute {
		globalNotifier.mu.Unlock()
		return // dedup
	}
	globalNotifier.cooldown[errKey] = time.Now()
	globalNotifier.mu.Unlock()

	msg := buildErrorMessage(params)
	go func() {
		if err := globalNotifier.sender(msg); err != nil {
			log.Printf("[kim] error notification failed: %v", err)
		}
	}()
}

// IsCancellationError reports whether the error string looks like a user or
// client-initiated cancellation. It intentionally does NOT match
// "context deadline exceeded" — that one is a real timeout we still want to
// know about.
func IsCancellationError(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	triggers := []string{
		"context canceled",
		"context cancelled",
		"net/http: request canceled",
		"net/http: request cancelled",
		"operation was canceled",
		"operation was cancelled",
		"client disconnected",
		"use of closed network connection",
	}
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// ErrorParams holds context for an error notification.
type ErrorParams struct {
	Component string // e.g. "MCP", "Stream", "Resume"
	Tool      string // tool name if applicable
	SessionID string
	RunID     string
	UserID    string
	Input     string // tool arguments or user message (truncated)
	Error     string // error message
	Retries   int    // number of retries attempted
}

func buildErrorMessage(p ErrorParams) string {
	now := time.Now().Format("2006-01-02 15:04:05")
	var sb strings.Builder
	sb.WriteString("# <font color=\"red\">**Knsight 错误告警**</font>\n\n")
	sb.WriteString(fmt.Sprintf("**时间**: %s\n\n", now))
	sb.WriteString(fmt.Sprintf("**组件**: %s\n\n", p.Component))
	if p.Tool != "" {
		sb.WriteString(fmt.Sprintf("**工具**: %s\n\n", p.Tool))
	}
	if p.UserID != "" {
		sb.WriteString(fmt.Sprintf("**用户**: %s\n\n", p.UserID))
	}
	if p.SessionID != "" {
		sb.WriteString(fmt.Sprintf("**会话**: `%s`\n\n", p.SessionID))
	}
	if p.RunID != "" {
		sb.WriteString(fmt.Sprintf("**RunID**: `%s`\n\n", p.RunID))
	}
	if p.Retries > 0 {
		sb.WriteString(fmt.Sprintf("**重试次数**: %d\n\n", p.Retries))
	}
	if p.Input != "" {
		input := p.Input
		if len(input) > 500 {
			input = input[:497] + "..."
		}
		sb.WriteString(fmt.Sprintf("**输入**: %s\n\n", input))
	}
	errMsg := p.Error
	if len(errMsg) > 1000 {
		errMsg = errMsg[:997] + "..."
	}
	sb.WriteString(fmt.Sprintf("**错误**: %s\n", errMsg))
	return sb.String()
}
