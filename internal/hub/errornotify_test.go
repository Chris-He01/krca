package hub

import "testing"

func TestIsCancellationError(t *testing.T) {
	cancelled := []string{
		// The exact strings we have seen in production alerts.
		`Post "https://llm.example.com/v1/chat/completions": context canceled`,
		"context canceled",
		"context cancelled",
		"net/http: request canceled",
		"operation was canceled",
		"client disconnected mid-stream",
		"read tcp 1.2.3.4:443: use of closed network connection",
		"Get \"http://example/\": context canceled",
	}
	for _, s := range cancelled {
		if !IsCancellationError(s) {
			t.Errorf("expected %q to be classified as cancellation", s)
		}
	}

	keepAlerting := []string{
		"",
		"context deadline exceeded", // explicit timeout, want to know
		"500 Internal Server Error",
		"failed to dial: connection refused",
		"messages.1: tool_use ids were found without tool_result blocks",
	}
	for _, s := range keepAlerting {
		if IsCancellationError(s) {
			t.Errorf("expected %q to be a real error (not cancellation)", s)
		}
	}
}
