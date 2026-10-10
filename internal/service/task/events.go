package task

import "movieagent/internal/service/sequence"

// Event mirrors agent_loop / UI stream events for the task scheduler.
type Event struct {
	Type       string
	TaskID     string
	TurnID     string
	SkillRunID string
	ToolCallID string
	ToolName   string
	Phase      string
	Delta      string
	Message    string
	IsError    bool
	Data       map[string]any
}

// Emitter receives lifecycle events (may be nil).
type Emitter func(Event)

func (s *Scheduler) emit(ev Event) {
	if s == nil || s.Emit == nil {
		return
	}
	s.Emit(ev)
}

func mapTaskStart(taskID string) Event {
	return Event{Type: sequence.AgentStart, TaskID: taskID, Phase: "agent"}
}

func mapTaskEnd(taskID string, data map[string]any) Event {
	return Event{Type: sequence.AgentEnd, TaskID: taskID, Phase: "agent", Data: data}
}

func mapTaskSettled(taskID string) Event {
	return Event{Type: sequence.AgentSettled, TaskID: taskID, Phase: "agent"}
}

func mapTurnStart(taskID, turnID string) Event {
	return Event{Type: sequence.TurnStart, TaskID: taskID, TurnID: turnID, Phase: "turn"}
}

func mapTurnEnd(taskID, turnID string) Event {
	return Event{Type: sequence.TurnEnd, TaskID: taskID, TurnID: turnID, Phase: "turn"}
}

func mapSkillStart(sk *SkillRun) Event {
	return Event{
		Type:       sequence.SkillStart,
		TaskID:     sk.TaskID,
		TurnID:     sk.TurnID,
		SkillRunID: sk.ID,
		ToolCallID: sk.ID,
		ToolName:   sk.Name,
		Phase:      "skill",
	}
}

func mapSkillEnd(sk *SkillRun) Event {
	return Event{
		Type:       sequence.SkillEnd,
		TaskID:     sk.TaskID,
		TurnID:     sk.TurnID,
		SkillRunID: sk.ID,
		ToolCallID: sk.ID,
		ToolName:   sk.Name,
		Phase:      "skill",
		IsError:    sk.IsError,
		Message:    sk.Result,
		Data:       sk.Data,
	}
}

func mapToolStart(sk *SkillRun) Event {
	return Event{
		Type:       sequence.ToolExecutionStart,
		TaskID:     sk.TaskID,
		TurnID:     sk.TurnID,
		SkillRunID: sk.ID,
		ToolCallID: sk.ID,
		ToolName:   sk.Name,
		Phase:      "tool",
	}
}

func mapToolEnd(sk *SkillRun) Event {
	return Event{
		Type:       sequence.ToolExecutionEnd,
		TaskID:     sk.TaskID,
		TurnID:     sk.TurnID,
		SkillRunID: sk.ID,
		ToolCallID: sk.ID,
		ToolName:   sk.Name,
		Phase:      "tool",
		IsError:    sk.IsError,
		Message:    sk.Result,
		Data:       sk.Data,
	}
}
