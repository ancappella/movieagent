package context

import (
	"encoding/json"
	"strings"
	"time"
)

// MessageKind is the session transcript role.
type MessageKind string

const (
	KindSystem     MessageKind = "system"
	KindUser       MessageKind = "user"
	KindAssistant  MessageKind = "assistant"
	KindToolResult MessageKind = "tool_result"
)

// ContentPart is one multimodal fragment inside a UserMessage.
type ContentPart struct {
	Type     string `json:"type"` // text | image
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	MIME     string `json:"mime,omitempty"`
}

// ToolCallReq is an AssistantMessage tool invocation request.
type ToolCallReq struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is one entry in session history.
//
//  1. SystemMessage   — session-level instructions / skills / tools / workdir; patchable by section
//  2. UserMessage     — user (or extension) input; multimodal parts; triggers the agent loop
//  3. AssistantMessage— LLM output: thinking text + toolCall requests
//  4. ToolResultMessage — tool stdout/error for one toolCall, fed back for the next think/act turn
type Message struct {
	ID        string      `json:"id,omitempty"`
	Kind      MessageKind `json:"kind"`
	CreatedAt time.Time   `json:"created_at,omitempty"`

	// SystemMessage: named sections (instructions, skills, tools, workdir, schedule, …).
	// Replay rebuilds Content from Sections in stable order.
	Sections map[string]string `json:"sections,omitempty"`

	// UserMessage multimodal parts (text/image). If empty, Content is used as plain text.
	Parts []ContentPart `json:"parts,omitempty"`

	// Shared text body (User plain text, Assistant reply, ToolResult payload, or rendered System).
	Content string `json:"content,omitempty"`

	// AssistantMessage
	Thinking  string        `json:"thinking,omitempty"`
	ToolCalls []ToolCallReq `json:"tool_calls,omitempty"`

	// ToolResultMessage
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
}

// System section keys (stable order when rendering).
const (
	SectionInstructions = "instructions"
	SectionWorkDir      = "workdir"
	SectionSkills       = "skills"
	SectionTools        = "tools"
	SectionSchedule     = "schedule"
	SectionExtra        = "extra"
)

var systemSectionOrder = []string{
	SectionInstructions,
	SectionWorkDir,
	SectionSchedule,
	SectionSkills,
	SectionTools,
	SectionExtra,
}

// RenderSystem joins named sections into one system prompt body.
func RenderSystem(sections map[string]string) string {
	if len(sections) == 0 {
		return ""
	}
	var b strings.Builder
	seen := map[string]bool{}
	for _, key := range systemSectionOrder {
		body := strings.TrimSpace(sections[key])
		if body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(body)
		seen[key] = true
	}
	for key, body := range sections {
		if seen[key] {
			continue
		}
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(body)
	}
	return strings.TrimSpace(b.String())
}

// UserText flattens user parts / content into a single text string for providers
// that only accept plain content (images stay as URL hints in text for now).
func (m Message) UserText() string {
	if len(m.Parts) == 0 {
		return strings.TrimSpace(m.Content)
	}
	var b strings.Builder
	for _, p := range m.Parts {
		switch strings.ToLower(strings.TrimSpace(p.Type)) {
		case "image":
			url := strings.TrimSpace(p.ImageURL)
			if url != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString("[image] ")
				b.WriteString(url)
			}
		default:
			t := strings.TrimSpace(p.Text)
			if t == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(t)
		}
	}
	if b.Len() == 0 {
		return strings.TrimSpace(m.Content)
	}
	return b.String()
}

// AssistantText prefers Content, else Thinking (for providers that lack a thinking channel).
func (m Message) AssistantText() string {
	if t := strings.TrimSpace(m.Content); t != "" {
		return t
	}
	return strings.TrimSpace(m.Thinking)
}

// MarshalArguments returns tool arguments as a JSON object string.
func MarshalArguments(v any) string {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return "{}"
		}
		if json.Valid([]byte(t)) {
			return t
		}
		b, _ := json.Marshal(t)
		return string(b)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "{}"
		}
		return string(b)
	}
}
