package herness

import (
	"encoding/json"
	"time"
)

// Role identifies a message author in the agent transcript.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// Message is one turn entry in ThreadState.Messages.
type Message struct {
	ID         string         `json:"id,omitempty"`
	Role       Role           `json:"role"`
	Content    string         `json:"content,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
}

// NewUserMessage builds a user message.
func NewUserMessage(content string) Message {
	return Message{ID: newID("msg"), Role: RoleUser, Content: content}
}

// NewSystemMessage builds a system message.
func NewSystemMessage(content string) Message {
	return Message{ID: newID("msg"), Role: RoleSystem, Content: content}
}

// NewAssistantMessage builds an assistant message, optionally with tool calls.
func NewAssistantMessage(content string, calls ...ToolCall) Message {
	return Message{ID: newID("msg"), Role: RoleAssistant, Content: content, ToolCalls: calls}
}

// NewToolMessage builds a tool result message.
func NewToolMessage(toolCallID, name, content string) Message {
	return Message{
		ID:         newID("msg"),
		Role:       RoleTool,
		Name:       name,
		ToolCallID: toolCallID,
		Content:    content,
	}
}

// ThreadState is the durable per-thread agent state (DeerFlow ThreadState subset).
type ThreadState struct {
	Messages   []Message      `json:"messages"`
	SandboxID  string         `json:"sandbox_id,omitempty"`
	Title      string         `json:"title,omitempty"`
	Artifacts  []string       `json:"artifacts,omitempty"`
	Summary    string         `json:"summary_text,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
}

// Clone returns a shallow copy with a copied message slice.
func (s ThreadState) Clone() ThreadState {
	out := s
	if s.Messages != nil {
		out.Messages = append([]Message(nil), s.Messages...)
	}
	if s.Artifacts != nil {
		out.Artifacts = append([]string(nil), s.Artifacts...)
	}
	if s.Extra != nil {
		out.Extra = make(map[string]any, len(s.Extra))
		for k, v := range s.Extra {
			out.Extra[k] = v
		}
	}
	return out
}

// AppendMessage appends messages and returns the updated state.
func (s ThreadState) AppendMessage(msgs ...Message) ThreadState {
	s.Messages = append(s.Messages, msgs...)
	s.UpdatedAt = time.Now().UTC()
	return s
}

// StreamEventType is the SDK stream event kind (DeerFlow StreamEvent).
type StreamEventType string

const (
	EventValues   StreamEventType = "values"
	EventMessages StreamEventType = "messages-tuple"
	EventCustom   StreamEventType = "custom"
	EventEnd      StreamEventType = "end"
	EventError    StreamEventType = "error"
)

// StreamEvent is one item yielded by Client.Stream.
type StreamEvent struct {
	Type StreamEventType `json:"type"`
	Data map[string]any  `json:"data"`
}

// RunConfig is per-turn runtime context (not checkpointed).
type RunConfig struct {
	ThreadID  string
	UserID    string
	RunID     string
	MaxSteps  int
	ExtraEnv  map[string]string
	AuthRole  string
	Values    map[string]any
}

// DefaultMaxSteps caps the model ↔ tools loop for a single user turn.
const DefaultMaxSteps = 16
