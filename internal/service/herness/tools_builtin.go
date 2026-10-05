package herness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// EchoTool echoes its text argument. Useful for demos and tests.
type EchoTool struct{}

func (EchoTool) Name() string { return "echo" }

func (EchoTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "echo",
		Description: "Echo back the provided text. Use for simple verification.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "Text to echo"},
			},
			"required": []string{"text"},
		},
	}
}

func (EchoTool) Run(_ context.Context, args json.RawMessage) (ToolResult, error) {
	var in struct {
		Text string `json:"text"`
	}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &in)
	}
	if in.Text == "" {
		in.Text = strings.TrimSpace(string(args))
	}
	return ToolResult{Content: in.Text}, nil
}

// AskClarificationTool asks the user a clarifying question and ends the turn.
type AskClarificationTool struct{}

func (AskClarificationTool) Name() string { return "ask_clarification" }

func (AskClarificationTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "ask_clarification",
		Description: "Ask the user a clarifying question when required information is missing.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string"},
			},
			"required": []string{"question"},
		},
	}
}

func (AskClarificationTool) Run(_ context.Context, args json.RawMessage) (ToolResult, error) {
	var in struct {
		Question string `json:"question"`
	}
	_ = json.Unmarshal(args, &in)
	if in.Question == "" {
		in.Question = "Could you clarify your request?"
	}
	return ToolResult{
		Content: in.Question,
		Data:    map[string]any{"clarification": true, "question": in.Question},
	}, nil
}

// BuiltinTools returns the default SDK tool set.
func BuiltinTools() []Tool {
	return []Tool{EchoTool{}, AskClarificationTool{}}
}

// FormatToolError turns a tool failure into content the model can read.
func FormatToolError(name string, err error) string {
	if err == nil {
		return fmt.Sprintf("tool %s failed", name)
	}
	return fmt.Sprintf("tool %s error: %v", name, err)
}
