package tools

import (
	"context"
	"fmt"
	"sync"

	"movieagent/internal/service/claw/schema"
)

// Handler executes one tool. arguments is the model JSON string.
type Handler func(ctx context.Context, arguments string) schema.ToolResult

// Gate optionally rate-limits Execute. release must be called once after the handler returns.
type Gate func(ctx context.Context, name string) (release func(), err error)

// Registry binds tool schemas (for the LLM) with handlers (for runtime).
type Registry struct {
	mu       sync.RWMutex
	defs     map[string]schema.ToolDefinition
	handlers map[string]Handler
	// Gate, when set, runs before each Execute (skill/tool dispatch limits).
	Gate Gate
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		defs:     make(map[string]schema.ToolDefinition),
		handlers: make(map[string]Handler),
	}
}

// Register binds a definition to its handler.
func (r *Registry) Register(def schema.ToolDefinition, h Handler) error {
	if def.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if h == nil {
		return fmt.Errorf("tool handler is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.defs[def.Name]; exists {
		return fmt.Errorf("tool already registered: %s", def.Name)
	}
	r.defs[def.Name] = def
	r.handlers[def.Name] = h
	return nil
}

// MustRegister panics on register failure (boot-time wiring).
func (r *Registry) MustRegister(def schema.ToolDefinition, h Handler) {
	if err := r.Register(def, h); err != nil {
		panic(err)
	}
}

// GetAvailableTools returns all registered tool definitions.
func (r *Registry) GetAvailableTools() []schema.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]schema.ToolDefinition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}

// Execute dispatches one tool call.
func (r *Registry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	r.mu.RLock()
	h, ok := r.handlers[call.Name]
	gate := r.Gate
	r.mu.RUnlock()
	if !ok {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    fmt.Sprintf("unknown tool: %s", call.Name),
			IsError:    true,
		}
	}
	if gate != nil {
		release, err := gate(ctx, call.Name)
		if err != nil {
			return schema.ToolResult{
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    fmt.Sprintf("dispatch limit: %v", err),
				IsError:    true,
			}
		}
		if release != nil {
			defer release()
		}
	}
	res := h(ctx, call.Arguments)
	if res.ToolCallID == "" {
		res.ToolCallID = call.ID
	}
	if res.Name == "" {
		res.Name = call.Name
	}
	return res
}

// ExecuteAll runs tool calls sequentially.
func (r *Registry) ExecuteAll(ctx context.Context, calls []schema.ToolCall) []schema.ToolResult {
	results := make([]schema.ToolResult, 0, len(calls))
	for _, c := range calls {
		results = append(results, r.Execute(ctx, c))
	}
	return results
}
