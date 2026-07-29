package hub

import "sync/atomic"

// SanitizerStatsSnapshot captures observability counters for
// sanitizeDanglingToolCalls. Each counter is monotonically increasing for
// the lifetime of the process; deltas are taken at the dashboard layer.
type SanitizerStatsSnapshot struct {
	// Runs is the number of times sanitizer changed at least one message.
	Runs int64 `json:"runs"`
	// Stripped counts assistant messages whose tool_calls were dropped
	// because the call IDs were missing (couldn't synthesize a placeholder).
	Stripped int64 `json:"stripped"`
	// Synthesized counts placeholder tool_result messages we appended to
	// repair an orphan tool_use. This is the preferred recovery mode.
	Synthesized int64 `json:"synthesized"`
	// OrphanConverted counts standalone Tool messages (no parent tool_use)
	// that we converted into annotated user context.
	OrphanConverted int64 `json:"orphan_converted"`
}

var (
	sanitizerRuns            atomic.Int64
	sanitizerStripped        atomic.Int64
	sanitizerSynthesized     atomic.Int64
	sanitizerOrphanConverted atomic.Int64
)

func recordSanitizer(stripped, synthesized, orphanConverted int) {
	sanitizerRuns.Add(1)
	if stripped > 0 {
		sanitizerStripped.Add(int64(stripped))
	}
	if synthesized > 0 {
		sanitizerSynthesized.Add(int64(synthesized))
	}
	if orphanConverted > 0 {
		sanitizerOrphanConverted.Add(int64(orphanConverted))
	}
}

// GetSanitizerStats returns a snapshot of the counters.
func GetSanitizerStats() SanitizerStatsSnapshot {
	return SanitizerStatsSnapshot{
		Runs:            sanitizerRuns.Load(),
		Stripped:        sanitizerStripped.Load(),
		Synthesized:     sanitizerSynthesized.Load(),
		OrphanConverted: sanitizerOrphanConverted.Load(),
	}
}

// resetSanitizerStatsForTest is used by unit tests to start from zero.
func resetSanitizerStatsForTest() {
	sanitizerRuns.Store(0)
	sanitizerStripped.Store(0)
	sanitizerSynthesized.Store(0)
	sanitizerOrphanConverted.Store(0)
}
