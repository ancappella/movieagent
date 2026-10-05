package herness

import (
	"context"
	"encoding/json"
)

// SandboxExecutor is the subset of sandbox APIs the harness needs.
type SandboxExecutor interface {
	ExecuteCommand(ctx context.Context, command string, env map[string]string) (string, error)
}

// BashTool runs a shell command inside an injected sandbox executor.
type BashTool struct {
	Sandbox SandboxExecutor
	Env     map[string]string
}

func (BashTool) Name() string { return "bash" }

func (t BashTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "bash",
		Description: "Execute a shell command in the thread sandbox.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required": []string{"command"},
		},
	}
}

func (t BashTool) Run(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.Sandbox == nil {
		return ToolResult{Error: "no_sandbox", Content: "bash tool has no sandbox"}, nil
	}
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return ToolResult{Error: err.Error(), Content: FormatToolError("bash", err)}, nil
	}
	out, err := t.Sandbox.ExecuteCommand(ctx, in.Command, t.Env)
	if err != nil {
		return ToolResult{Error: err.Error(), Content: FormatToolError("bash", err)}, nil
	}
	return ToolResult{Content: out}, nil
}
