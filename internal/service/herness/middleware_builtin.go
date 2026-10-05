package herness

import (
	"context"
	"fmt"
	"time"
)

// ToolErrorMiddleware converts tool panics/errors into ToolResult content
// so the model loop can continue (DeerFlow ToolErrorHandlingMiddleware).
type ToolErrorMiddleware struct{ BaseMiddleware }

func (ToolErrorMiddleware) Name() string { return "tool_error" }

func (ToolErrorMiddleware) WrapToolCall(ctx context.Context, call ToolCall, next ToolHandler) (res ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = ToolResult{Error: fmt.Sprintf("panic: %v", r), Content: FormatToolError(call.Name, fmt.Errorf("%v", r))}
			err = nil
		}
	}()
	res, err = next(ctx, call)
	if err != nil {
		return ToolResult{Error: err.Error(), Content: FormatToolError(call.Name, err)}, nil
	}
	if res.Error != "" && res.Content == "" {
		res.Content = FormatToolError(call.Name, fmt.Errorf("%s", res.Error))
	}
	return res, nil
}

// DanglingToolCallMiddleware fills missing tool results before a model call
// when a previous assistant message still has unanswered tool_calls.
type DanglingToolCallMiddleware struct{ BaseMiddleware }

func (DanglingToolCallMiddleware) Name() string { return "dangling_tool_call" }

func (m DanglingToolCallMiddleware) BeforeAgent(_ context.Context, state *ThreadState, _ *RunConfig) error {
	if state == nil || len(state.Messages) == 0 {
		return nil
	}
	answered := map[string]struct{}{}
	for _, msg := range state.Messages {
		if msg.Role == RoleTool && msg.ToolCallID != "" {
			answered[msg.ToolCallID] = struct{}{}
		}
	}
	var missing []Message
	for _, msg := range state.Messages {
		if msg.Role != RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.ID == "" {
				continue
			}
			if _, ok := answered[call.ID]; ok {
				continue
			}
			missing = append(missing, NewToolMessage(call.ID, call.Name, "tool call was cancelled or never completed"))
			answered[call.ID] = struct{}{}
		}
	}
	if len(missing) > 0 {
		state.Messages = append(state.Messages, missing...)
	}
	return nil
}

// DynamicContextMiddleware injects a short runtime preamble into the system prompt.
type DynamicContextMiddleware struct {
	BaseMiddleware
	Clock func() time.Time
}

func (DynamicContextMiddleware) Name() string { return "dynamic_context" }

func (m DynamicContextMiddleware) WrapModelCall(ctx context.Context, req *ModelRequest, next ModelHandler) (ModelResponse, error) {
	now := time.Now().UTC()
	if m.Clock != nil {
		now = m.Clock()
	}
	block := fmt.Sprintf("\n\n<runtime_context>\ncurrent_time_utc: %s\n</runtime_context>", now.Format(time.RFC3339))
	if req.SystemPrompt == "" {
		req.SystemPrompt = "You are a helpful agent." + block
	} else {
		req.SystemPrompt += block
	}
	return next(ctx, req)
}

// SandboxMiddleware records a sandbox id onto ThreadState when missing.
// The concrete provider is injected; nil provider is a no-op.
type SandboxMiddleware struct {
	BaseMiddleware
	Acquire func(ctx context.Context, run *RunConfig) (sandboxID string, err error)
}

func (SandboxMiddleware) Name() string { return "sandbox" }

func (m SandboxMiddleware) BeforeAgent(ctx context.Context, state *ThreadState, run *RunConfig) error {
	if state == nil || state.SandboxID != "" || m.Acquire == nil {
		return nil
	}
	id, err := m.Acquire(ctx, run)
	if err != nil {
		return err
	}
	state.SandboxID = id
	return nil
}

// DefaultMiddlewares builds the minimal runtime chain used by NewAgent when
// Options.Middleware is empty and Features are applied.
func DefaultMiddlewares() []Middleware {
	return []Middleware{
		ToolErrorMiddleware{},
		DanglingToolCallMiddleware{},
		DynamicContextMiddleware{},
	}
}
