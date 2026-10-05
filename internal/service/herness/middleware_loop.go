package herness

import (
	"context"
	"fmt"
)

// LoopDetectionMiddleware stops repeated identical tool calls in one turn.
// Use as a pointer so per-turn counts persist across WrapToolCall invocations.
type LoopDetectionMiddleware struct {
	BaseMiddleware
	MaxSameTool int
	counts      map[string]int
}

func (m *LoopDetectionMiddleware) Name() string { return "loop_detection" }

func (m *LoopDetectionMiddleware) BeforeAgent(context.Context, *ThreadState, *RunConfig) error {
	m.counts = map[string]int{}
	return nil
}

func (m *LoopDetectionMiddleware) WrapToolCall(ctx context.Context, call ToolCall, next ToolHandler) (ToolResult, error) {
	if m.counts == nil {
		m.counts = map[string]int{}
	}
	limit := m.MaxSameTool
	if limit <= 0 {
		limit = 4
	}
	key := call.Name + ":" + string(call.Args)
	m.counts[key]++
	if m.counts[key] > limit {
		return ToolResult{
			Content: fmt.Sprintf("loop detected: tool %s called %d times with the same args; refusing further calls", call.Name, m.counts[key]),
			Error:   "loop_detected",
		}, nil
	}
	return next(ctx, call)
}
