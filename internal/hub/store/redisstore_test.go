package store

import "testing"

func TestLLMRoundRedisID(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		runID     string
		want      string
	}{
		{
			name:      "session and run",
			sessionID: "session-123",
			runID:     "run-456",
			want:      "session-123:run-456",
		},
		{
			name:  "legacy run only",
			runID: "run-456",
			want:  "run-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := llmRoundRedisID(tt.sessionID, tt.runID); got != tt.want {
				t.Fatalf("llmRoundRedisID() = %q, want %q", got, tt.want)
			}
		})
	}
}
