package hub

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// loopDetectorThreshold is the number of consecutive identical tool calls
// before we inject a loop-break message.
const loopDetectorThreshold = 5

// NewLoopDetector returns a BeforeChatModel callback that detects repeated
// tool call patterns. When the last N assistant messages all share the same
// combined tool-call signature (all function names + args joined), it appends
// a user message telling the LLM to break out.
// All ToolCalls in each message are included so that multi-tool loops are
// caught, not just single-tool repetitions.
func NewLoopDetector() func(context.Context, *adk.ChatModelAgentState) error {
	return func(_ context.Context, state *adk.ChatModelAgentState) error {
		msgs := state.Messages
		if len(msgs) < loopDetectorThreshold*2 {
			return nil
		}

		var recent []string
		for i := len(msgs) - 1; i >= 0 && len(recent) < loopDetectorThreshold; i-- {
			m := msgs[i]
			if m.Role != schema.Assistant || len(m.ToolCalls) == 0 {
				continue
			}
			var parts []string
			for _, tc := range m.ToolCalls {
				if tc.Function.Name == "" {
					continue
				}
				parts = append(parts, tc.Function.Name+"@"+tc.Function.Arguments)
			}
			if len(parts) == 0 {
				continue
			}
			recent = append(recent, strings.Join(parts, "|"))
		}

		if len(recent) < loopDetectorThreshold {
			return nil
		}

		first := recent[0]
		for _, r := range recent[1:] {
			if r != first {
				return nil
			}
		}

		displayName := first
		if idx := strings.Index(first, "@"); idx >= 0 {
			displayName = first[:idx]
		}
		log.Printf("[loop-detector] detected %d consecutive identical calls to %s, injecting break message", loopDetectorThreshold, displayName)

		state.Messages = append(state.Messages, schema.UserMessage(
			fmt.Sprintf("[LOOP DETECTED] You have called tool '%s' with identical arguments %d times consecutively. "+
				"This indicates you are stuck in a loop. STOP calling this tool immediately. "+
				"Instead, summarize whatever data you have collected so far and return your results to the caller. "+
				"If you lack sufficient data, explain what is missing and return.",
				displayName, loopDetectorThreshold),
		))

		return nil
	}
}
