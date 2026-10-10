package tools

import (
	"context"
	"encoding/json"

	"movieagent/internal/service/claw/schema"
	"movieagent/internal/service/herness"
)

// RegisterHarness mounts herness.Tool instances onto the claw registry.
func RegisterHarness(reg *Registry, list ...herness.Tool) {
	if reg == nil {
		return
	}
	for _, t := range list {
		if t == nil || t.Name() == "" {
			continue
		}
		tool := t
		s := tool.Schema()
		_ = reg.Register(schema.ToolDefinition{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.Parameters,
		}, func(ctx context.Context, arguments string) schema.ToolResult {
			raw := json.RawMessage(arguments)
			if !json.Valid(raw) {
				raw = json.RawMessage([]byte("{}"))
			}
			res, err := tool.Run(ctx, raw)
			if err != nil {
				return schema.ToolResult{Content: err.Error(), IsError: true}
			}
			out := schema.ToolResult{
				Name:    tool.Name(),
				Content: res.Content,
				Data:    res.Data,
			}
			if res.Error != "" {
				out.IsError = true
				if out.Content == "" {
					out.Content = res.Error
				}
			}
			if out.Content == "" && !out.IsError {
				out.Content = "ok"
			}
			return out
		})
	}
}
