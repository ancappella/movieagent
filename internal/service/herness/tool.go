package herness

import (
	"context"
	"encoding/json"
)

// ToolSchema is the JSON-schema style description passed to the model.
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolResult is the outcome of one tool invocation.
type ToolResult struct {
	Content string
	Error   string
	Data    map[string]any
}

// Tool is a callable capability exposed to the agent.
type Tool interface {
	Name() string
	Schema() ToolSchema
	Run(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ToolFunc adapts a function into a Tool.
type ToolFunc struct {
	ToolName string
	ToolDesc string
	Params   map[string]any
	Fn       func(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

func (t ToolFunc) Name() string { return t.ToolName }

func (t ToolFunc) Schema() ToolSchema {
	return ToolSchema{Name: t.ToolName, Description: t.ToolDesc, Parameters: t.Params}
}

func (t ToolFunc) Run(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.Fn == nil {
		return ToolResult{Error: "tool has no handler"}, nil
	}
	return t.Fn(ctx, args)
}

// Registry holds tools by name.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry builds a registry from tools. Later tools with the same name replace earlier ones.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range tools {
		if t == nil || t.Name() == "" {
			continue
		}
		if _, ok := r.tools[t.Name()]; !ok {
			r.order = append(r.order, t.Name())
		}
		r.tools[t.Name()] = t
	}
	return r
}

func (r *Registry) Get(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	if r == nil {
		return nil
	}
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		if t, ok := r.tools[name]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) Schemas() []ToolSchema {
	tools := r.List()
	out := make([]ToolSchema, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Schema())
	}
	return out
}
