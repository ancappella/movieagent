package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"movieagent/internal/service/claw/schema"
)

// RegisterBuiltins mounts the demo tools from go-claw 04 (echo, add).
func RegisterBuiltins(reg *Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(schema.ToolDefinition{
		Name:        "echo",
		Description: "Echo back the given text",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "Text to echo"},
			},
			"required": []string{"text"},
		},
	}, func(_ context.Context, arguments string) schema.ToolResult {
		var args struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return schema.ToolResult{Content: err.Error(), IsError: true}
		}
		return schema.ToolResult{Content: args.Text}
	})

	reg.MustRegister(schema.ToolDefinition{
		Name:        "add",
		Description: "Add two integers and return the sum as a string",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "integer", "description": "First addend"},
				"b": map[string]any{"type": "integer", "description": "Second addend"},
			},
			"required": []string{"a", "b"},
		},
	}, func(_ context.Context, arguments string) schema.ToolResult {
		var args struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return schema.ToolResult{Content: err.Error(), IsError: true}
		}
		return schema.ToolResult{Content: fmt.Sprintf("%d", args.A+args.B)}
	})
}
