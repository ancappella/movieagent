package herness

import (
	"context"
	"encoding/json"
	"fmt"
)

// TurnResult is the outcome of one user turn through the main loop.
type TurnResult struct {
	State    ThreadState
	Response string
	Steps    int
}

// Runner executes the model ↔ tools main loop.
type Runner struct {
	Agent *Agent
}

func NewRunner(agent *Agent) *Runner {
	return &Runner{Agent: agent}
}

// RunTurn appends the user message and runs until the model stops calling tools
// or MaxSteps is reached. Events are optional; pass nil to ignore streaming.
func (r *Runner) RunTurn(ctx context.Context, run RunConfig, userText string, emit func(StreamEvent)) (TurnResult, error) {
	if r == nil || r.Agent == nil {
		return TurnResult{}, fmt.Errorf("herness: runner has no agent")
	}
	if emit == nil {
		emit = func(StreamEvent) {}
	}
	if run.ThreadID == "" {
		run.ThreadID = newID("thread")
	}
	if run.RunID == "" {
		run.RunID = newID("run")
	}
	maxSteps := run.MaxSteps
	if maxSteps <= 0 {
		maxSteps = r.Agent.MaxSteps
	}

	state, err := r.loadState(ctx, run.ThreadID)
	if err != nil {
		return TurnResult{}, err
	}
	state = state.AppendMessage(NewUserMessage(userText))

	if err := r.Agent.Middleware.beforeAgent(ctx, &state, &run); err != nil {
		return TurnResult{}, err
	}

	emit(StreamEvent{Type: EventValues, Data: map[string]any{
		"thread_id":  run.ThreadID,
		"run_id":     run.RunID,
		"sandbox_id": state.SandboxID,
		"messages":   len(state.Messages),
	}})

	modelCall := r.Agent.Middleware.model(func(ctx context.Context, req *ModelRequest) (ModelResponse, error) {
		return r.Agent.Model.Chat(ctx, *req)
	})
	toolCall := r.Agent.Middleware.tool(func(ctx context.Context, call ToolCall) (ToolResult, error) {
		tool, ok := r.Agent.Tools.Get(call.Name)
		if !ok {
			return ToolResult{Error: "unknown_tool", Content: fmt.Sprintf("unknown tool: %s", call.Name)}, nil
		}
		return tool.Run(ctx, call.Args)
	})

	var final string
	steps := 0
	for step := 0; step < maxSteps; step++ {
		steps = step + 1
		req := &ModelRequest{
			Messages:     append([]Message(nil), state.Messages...),
			Tools:        r.Agent.Tools.Schemas(),
			SystemPrompt: r.Agent.SystemPrompt,
		}
		resp, err := modelCall(ctx, req)
		if err != nil {
			emit(StreamEvent{Type: EventError, Data: map[string]any{"error": err.Error()}})
			return TurnResult{}, err
		}
		ai := resp.Message
		if ai.Role == "" {
			ai.Role = RoleAssistant
		}
		if ai.ID == "" {
			ai.ID = newID("msg")
		}
		state = state.AppendMessage(ai)
		emit(StreamEvent{Type: EventMessages, Data: map[string]any{
			"role":       string(ai.Role),
			"content":    ai.Content,
			"tool_calls": len(ai.ToolCalls),
			"id":         ai.ID,
		}})

		if len(ai.ToolCalls) == 0 {
			final = ai.Content
			break
		}

		stopAfterTools := false
		for _, call := range ai.ToolCalls {
			if call.ID == "" {
				call.ID = newID("call")
			}
			res, err := toolCall(ctx, call)
			if err != nil {
				res = ToolResult{Content: FormatToolError(call.Name, err), Error: err.Error()}
			}
			content := res.Content
			if content == "" && res.Error != "" {
				content = res.Error
			}
			tm := NewToolMessage(call.ID, call.Name, content)
			if res.Data != nil {
				tm.Extra = res.Data
			}
			state = state.AppendMessage(tm)
			emit(StreamEvent{Type: EventCustom, Data: map[string]any{
				"event":   "tool_result",
				"tool":    call.Name,
				"call_id": call.ID,
				"content": content,
				"error":   res.Error,
			}})
			if call.Name == "ask_clarification" || (res.Data != nil && res.Data["clarification"] == true) {
				final = content
				stopAfterTools = true
			}
		}
		if stopAfterTools {
			break
		}
	}

	if err := r.Agent.Middleware.afterAgent(ctx, &state, &run); err != nil {
		return TurnResult{}, err
	}
	if err := r.Agent.Checkpointer.Put(ctx, run.ThreadID, state); err != nil {
		return TurnResult{}, err
	}

	emit(StreamEvent{Type: EventEnd, Data: map[string]any{
		"thread_id": run.ThreadID,
		"run_id":    run.RunID,
		"steps":     steps,
		"response":  final,
	}})

	return TurnResult{State: state, Response: final, Steps: steps}, nil
}

func (r *Runner) loadState(ctx context.Context, threadID string) (ThreadState, error) {
	state, ok, err := r.Agent.Checkpointer.Get(ctx, threadID)
	if err != nil {
		return ThreadState{}, err
	}
	if !ok {
		return ThreadState{}, nil
	}
	return state, nil
}

// mustJSON is a tiny helper for tests/demos building tool args.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
