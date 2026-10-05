package agent_loop

import "time"

// Message is one transcript item (pi-agent AgentMessage subset).
type Message struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Name      string         `json:"name,omitempty"`
	ToolCallID string        `json:"toolCallId,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

func UserMessage(text string) Message {
	return Message{Role: "user", Content: text, Timestamp: time.Now().UnixMilli()}
}

func AssistantMessage(text string) Message {
	return Message{Role: "assistant", Content: text, Timestamp: time.Now().UnixMilli()}
}

func ToolResultMessage(id, name, content string) Message {
	return Message{
		Role:       "toolResult",
		Content:    content,
		Name:       name,
		ToolCallID: id,
		Timestamp:  time.Now().UnixMilli(),
	}
}

// Event matches pi-agent-core AgentEvent names so the UI can follow the same flow.
type Event struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`

	Message             *Message `json:"message,omitempty"`
	AssistantMessageEvent any    `json:"assistantMessageEvent,omitempty"`

	ToolCallID    string `json:"toolCallId,omitempty"`
	ToolName      string `json:"toolName,omitempty"`
	Args          any    `json:"args,omitempty"`
	PartialResult any    `json:"partialResult,omitempty"`
	Result        any    `json:"result,omitempty"`
	IsError       bool   `json:"isError,omitempty"`

	ToolResults []Message `json:"toolResults,omitempty"`
	Messages    []Message `json:"messages,omitempty"`
	Data        any       `json:"data,omitempty"`

	Phase     string `json:"phase,omitempty"`
	Skill     string `json:"skill,omitempty"`
	Delta     string `json:"delta,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
}

// Emit delivers one event to subscribers (SSE / NDJSON).
type Emit func(Event)
