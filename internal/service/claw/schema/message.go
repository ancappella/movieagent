// Package schema defines shared message and tool types for the claw chat engine
// (ported from go-claw/04-go-claw-tool-register).
package schema

// Role is a chat message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in the conversation history.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition is the schema exposed to the model.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolResult is the outcome of executing one ToolCall.
type ToolResult struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Content    string         `json:"content"`
	IsError    bool           `json:"is_error,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// ChatRequest is sent to the LLM provider.
// Tools == nil means Thinking phase (no tool schemas).
type ChatRequest struct {
	Messages []Message
	Tools    []ToolDefinition // nil = Thinking; non-nil (even empty) = Action
}

// ChatResponse is one provider completion.
type ChatResponse struct {
	Message      Message
	FinishReason string // stop | tool_calls | length | ...
}
