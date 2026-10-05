package agent_loop

import (
	"context"
	"fmt"

	"movieagent/internal/service/sequence"
)

// WorkFn runs one agent reasoning workflow (tools / nodes) and returns the
// payload shown to the UI on agent_end.
type WorkFn func(ctx context.Context, sess *Session, prompt string, emit Emit) (map[string]any, error)

// RunPrompt executes one pi-agent-style prompt() on an existing robot session:
//
//	agent_start → turn_start → user message_start/end → work (tool_execution_*)
//	→ assistant message_start/end → turn_end → agent_end → agent_settled
func RunPrompt(ctx context.Context, sess *Session, prompt string, emit Emit, work WorkFn) error {
	if sess == nil {
		return fmt.Errorf("robot session is nil")
	}
	if emit == nil {
		emit = func(Event) {}
	}
	wrap := func(ev Event) {
		ev.SessionID = sess.ID
		emit(ev)
	}

	wrap(Event{Type: sequence.AgentStart})
	wrap(Event{Type: sequence.TurnStart})

	user := UserMessage(prompt)
	sess.append(user)
	wrap(Event{Type: sequence.MessageStart, Message: &user})
	wrap(Event{Type: sequence.MessageEnd, Message: &user})

	var (
		data map[string]any
		err  error
	)
	if work != nil {
		wrap(Event{Type: sequence.ThinkingStart, Phase: "thinking", Thinking: "正在理解你的需求，并规划这一轮要做的事…"})
		data, err = work(ctx, sess, prompt, wrap)
		wrap(Event{Type: sequence.ThinkingEnd, Phase: "thinking"})
	}
	if err != nil {
		fail := AssistantMessage(err.Error())
		fail.Extra = map[string]any{"error": true}
		sess.append(fail)
		wrap(Event{Type: sequence.MessageStart, Message: &fail})
		wrap(Event{Type: sequence.MessageEnd, Message: &fail})
		wrap(Event{Type: sequence.TurnEnd, Message: &fail, ToolResults: nil})
		wrap(Event{Type: sequence.AgentEnd, Messages: sess.messagesCopy(), Data: map[string]any{"error": err.Error()}})
		wrap(Event{Type: sequence.AgentSettled})
		return err
	}

	text, _ := data["message"].(string)
	if text == "" {
		if t, ok := data["creative_text"].(string); ok {
			text = t
		}
	}
	assistant := AssistantMessage(text)
	sess.append(assistant)
	if data != nil {
		sess.setLast(data)
	}

	wrap(Event{Type: sequence.MessageStart, Message: &assistant})
	wrap(Event{Type: sequence.MessageUpdate, Message: &assistant, AssistantMessageEvent: map[string]any{"type": "text_delta", "delta": text}})
	wrap(Event{Type: sequence.MessageEnd, Message: &assistant})
	wrap(Event{Type: sequence.TurnEnd, Message: &assistant})
	wrap(Event{Type: sequence.AgentEnd, Messages: sess.messagesCopy(), Data: data})
	wrap(Event{Type: sequence.AgentSettled})
	return nil
}

// EmitToolStart / EmitToolEnd helpers for workflow nodes.
func EmitToolStart(emit Emit, id, name string, args any) {
	if emit == nil {
		return
	}
	emit(Event{Type: sequence.ToolExecutionStart, ToolCallID: id, ToolName: name, Args: args, Phase: "tool"})
}

func EmitToolEnd(emit Emit, id, name string, result any, isErr bool) {
	if emit == nil {
		return
	}
	emit(Event{Type: sequence.ToolExecutionEnd, ToolCallID: id, ToolName: name, Result: result, IsError: isErr, Phase: "tool"})
}

func EmitThinking(emit Emit, delta string) {
	if emit == nil || delta == "" {
		return
	}
	emit(Event{Type: sequence.ThinkingDelta, Phase: "thinking", Delta: delta, Thinking: delta})
}

func EmitSkillStart(emit Emit, skill, label string) {
	if emit == nil || skill == "" {
		return
	}
	emit(Event{Type: sequence.SkillStart, Phase: "skill", Skill: skill, ToolName: label, ToolCallID: skill})
}

func EmitSkillEnd(emit Emit, skill, label string, result any, isErr bool) {
	if emit == nil || skill == "" {
		return
	}
	emit(Event{Type: sequence.SkillEnd, Phase: "skill", Skill: skill, ToolName: label, ToolCallID: skill, Result: result, IsError: isErr})
}
