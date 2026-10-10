package agentruntime

import (
	"fmt"
	"strings"
	"time"

	agentctx "movieagent/internal/context"
	"movieagent/internal/service/claw/schema"
)

// FromSchema converts a provider/engine message into agentctx.Message.
func FromSchema(m schema.Message) agentctx.Message {
	now := time.Now().UTC()
	switch m.Role {
	case schema.RoleSystem:
		return agentctx.Message{
			ID:        fmt.Sprintf("sys-%d", now.UnixNano()),
			Kind:      agentctx.KindSystem,
			Content:   m.Content,
			CreatedAt: now,
			Sections:  map[string]string{agentctx.SectionInstructions: m.Content},
		}
	case schema.RoleUser:
		return agentctx.Message{
			ID:        fmt.Sprintf("user-%d", now.UnixNano()),
			Kind:      agentctx.KindUser,
			Content:   m.Content,
			CreatedAt: now,
		}
	case schema.RoleTool:
		return agentctx.Message{
			ID:         fmt.Sprintf("tool-%d", now.UnixNano()),
			Kind:       agentctx.KindToolResult,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolName:   m.Name,
			CreatedAt:  now,
		}
	default:
		calls := make([]agentctx.ToolCallReq, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			calls = append(calls, agentctx.ToolCallReq{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
		return agentctx.Message{
			ID:        fmt.Sprintf("asst-%d", now.UnixNano()),
			Kind:      agentctx.KindAssistant,
			Content:   m.Content,
			ToolCalls: calls,
			CreatedAt: now,
		}
	}
}

// FromSchemaHistory converts a full engine history slice.
func FromSchemaHistory(msgs []schema.Message) []agentctx.Message {
	out := make([]agentctx.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, FromSchema(m))
	}
	return out
}

// DiffNewMessages returns messages in after that are beyond the prefix length of before.
// Used to avoid re-seeding the entire history on every barrier.
func DiffNewMessages(before, after []schema.Message) []schema.Message {
	if len(after) <= len(before) {
		return nil
	}
	return after[len(before):]
}

// ToolStatesFromAssistant builds planned ToolCallState entries from an assistant message.
func ToolStatesFromAssistant(m schema.Message) []ToolCallState {
	if m.Role != schema.RoleAssistant || len(m.ToolCalls) == 0 {
		return nil
	}
	out := make([]ToolCallState, 0, len(m.ToolCalls))
	for _, tc := range m.ToolCalls {
		out = append(out, ToolCallState{
			ToolCallID: tc.ID,
			Name:       tc.Name,
			Arguments:  tc.Arguments,
			Status:     "planned",
			Replay:     DefaultReplayForTool(tc.Name),
		})
	}
	return out
}

// AnnotateThinking moves content into Thinking when phase is thinking.
func AnnotateThinking(msg agentctx.Message, phase string) agentctx.Message {
	if phase != PhaseThinking || msg.Kind != agentctx.KindAssistant {
		return msg
	}
	if strings.TrimSpace(msg.Thinking) == "" && strings.TrimSpace(msg.Content) != "" {
		msg.Thinking = msg.Content
	}
	return msg
}

func cloneMessage(m agentctx.Message) agentctx.Message {
	out := m
	if m.Sections != nil {
		out.Sections = make(map[string]string, len(m.Sections))
		for k, v := range m.Sections {
			out.Sections[k] = v
		}
	}
	if m.Parts != nil {
		out.Parts = append([]agentctx.ContentPart(nil), m.Parts...)
	}
	if m.ToolCalls != nil {
		out.ToolCalls = append([]agentctx.ToolCallReq(nil), m.ToolCalls...)
	}
	return out
}

func cloneMessages(msgs []agentctx.Message) []agentctx.Message {
	if msgs == nil {
		return nil
	}
	out := make([]agentctx.Message, len(msgs))
	for i, m := range msgs {
		out[i] = cloneMessage(m)
	}
	return out
}
