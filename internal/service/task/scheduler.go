package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func newID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

// Scheduler drives AgentTask → Turn → SkillRun with durable state transitions.
type Scheduler struct {
	Store Store
	Emit  Emitter
}

// NewScheduler builds a scheduler. Store must be non-nil.
func NewScheduler(store Store) *Scheduler {
	return &Scheduler{Store: store}
}

// Start records a pending AgentTask. Call Activate after dispatch grants a slot.
func (s *Scheduler) Start(ctx context.Context, req StartRequest) (*AgentTask, error) {
	if s == nil || s.Store == nil {
		return nil, fmt.Errorf("scheduler: store required")
	}
	now := time.Now().UTC()
	t := &AgentTask{
		ID:                  newID("task"),
		ThreadID:            strings.TrimSpace(req.ThreadID),
		SessionID:           strings.TrimSpace(req.SessionID),
		Goal:                strings.TrimSpace(req.Goal),
		Status:              StatusPending,
		Meta:                req.Meta,
		FailTaskOnTurnError: req.FailTaskOnTurnError,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.Store.SaveTask(ctx, t); err != nil {
		return nil, err
	}
	_ = s.checkpoint(ctx, t)
	return t, nil
}

// Activate moves a pending AgentTask to running when dispatch schedules it.
func (s *Scheduler) Activate(ctx context.Context, taskID string) (*AgentTask, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case StatusPending, StatusPaused:
		// ok
	case StatusRunning:
		return task, nil
	default:
		return nil, fmt.Errorf("task %s cannot activate from %s", taskID, task.Status)
	}
	now := time.Now().UTC()
	task.Status = StatusRunning
	task.UpdatedAt = now
	if task.StartedAt == nil {
		task.StartedAt = &now
	}
	if err := s.Store.SaveTask(ctx, task); err != nil {
		return nil, err
	}
	_ = s.checkpoint(ctx, task)
	s.emit(mapTaskStart(task.ID))
	return task, nil
}

// BeginTurn opens a new Turn under the task (pending→running).
func (s *Scheduler) BeginTurn(ctx context.Context, taskID, input string) (*Turn, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status.Terminal() {
		return nil, fmt.Errorf("task %s is %s", taskID, task.Status)
	}
	if task.Status == StatusPending {
		return nil, fmt.Errorf("task %s is pending; activate via dispatch first", taskID)
	}
	if task.Status == StatusPaused {
		task.Status = StatusRunning
	}
	turns, err := s.Store.ListTurns(ctx, taskID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	turn := &Turn{
		ID:        newID("turn"),
		TaskID:    taskID,
		Index:     len(turns),
		Status:    StatusRunning,
		Input:     input,
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	}
	if err := s.Store.SaveTurn(ctx, turn); err != nil {
		return nil, err
	}
	task.CurrentTurnID = turn.ID
	task.TurnIDs = append(task.TurnIDs, turn.ID)
	task.UpdatedAt = now
	if err := s.Store.SaveTask(ctx, task); err != nil {
		return nil, err
	}
	_ = s.checkpoint(ctx, task)
	s.emit(mapTurnStart(taskID, turn.ID))
	return turn, nil
}

// EndTurn closes a turn. skill failures do not fail the turn unless FailTurnOnSkillError.
func (s *Scheduler) EndTurn(ctx context.Context, turnID, thinking, assistantText string, turnErr error) (*Turn, error) {
	turn, err := s.Store.GetTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	turn.Thinking = thinking
	turn.AssistantText = assistantText
	turn.UpdatedAt = now
	turn.EndedAt = &now

	if turnErr != nil {
		turn.Status = StatusFailed
		turn.Error = turnErr.Error()
	} else if turn.FailTurnOnSkillError {
		skills, _ := s.Store.ListSkills(ctx, turnID)
		for _, sk := range skills {
			if sk.Status == StatusFailed || sk.IsError {
				turn.Status = StatusFailed
				turn.Error = "skill failed: " + sk.Name
				break
			}
		}
		if turn.Status != StatusFailed {
			turn.Status = StatusSucceeded
		}
	} else {
		turn.Status = StatusSucceeded
	}

	if err := s.Store.SaveTurn(ctx, turn); err != nil {
		return nil, err
	}

	task, err := s.Store.GetTask(ctx, turn.TaskID)
	if err != nil {
		return nil, err
	}
	task.UpdatedAt = now
	if turn.Status == StatusFailed && task.FailTaskOnTurnError {
		task.Status = StatusFailed
		task.Error = turn.Error
		task.EndedAt = &now
		_ = s.Store.SaveTask(ctx, task)
		s.emit(mapTurnEnd(task.ID, turn.ID))
		s.emit(mapTaskEnd(task.ID, map[string]any{"error": task.Error}))
		s.emit(mapTaskSettled(task.ID))
		return turn, turnErr
	}
	_ = s.Store.SaveTask(ctx, task)
	_ = s.checkpoint(ctx, task)
	s.emit(mapTurnEnd(task.ID, turn.ID))
	return turn, nil
}

// BeginSkill starts a SkillRun inside a turn (does not abort siblings on failure).
func (s *Scheduler) BeginSkill(ctx context.Context, turnID, name string, kind Kind, args any) (*SkillRun, error) {
	turn, err := s.Store.GetTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}
	if turn.Status.Terminal() {
		return nil, fmt.Errorf("turn %s is %s", turnID, turn.Status)
	}
	raw, _ := json.Marshal(args)
	if args == nil {
		raw = []byte("{}")
	}
	if kind == "" {
		kind = KindTool
	}
	now := time.Now().UTC()
	sk := &SkillRun{
		ID:        newID("skill"),
		TaskID:    turn.TaskID,
		TurnID:    turnID,
		Name:      strings.TrimSpace(name),
		Kind:      kind,
		Args:      raw,
		Status:    StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	}
	if err := s.Store.SaveSkill(ctx, sk); err != nil {
		return nil, err
	}
	turn.SkillRunIDs = append(turn.SkillRunIDs, sk.ID)
	turn.UpdatedAt = now
	if err := s.Store.SaveTurn(ctx, turn); err != nil {
		return nil, err
	}
	if kind == KindSkill {
		s.emit(mapSkillStart(sk))
	}
	s.emit(mapToolStart(sk))
	return sk, nil
}

// EndSkill records skill/tool completion. Failure is isolated to this SkillRun by default.
func (s *Scheduler) EndSkill(ctx context.Context, skillRunID, result string, data map[string]any, runErr error) (*SkillRun, error) {
	sk, err := s.Store.GetSkill(ctx, skillRunID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sk.Result = result
	sk.Data = data
	sk.UpdatedAt = now
	sk.EndedAt = &now
	if runErr != nil {
		sk.Status = StatusFailed
		sk.IsError = true
		sk.Error = runErr.Error()
		if sk.Result == "" {
			sk.Result = runErr.Error()
		}
	} else {
		sk.Status = StatusSucceeded
		sk.IsError = false
	}
	if err := s.Store.SaveSkill(ctx, sk); err != nil {
		return nil, err
	}
	s.emit(mapToolEnd(sk))
	if sk.Kind == KindSkill {
		s.emit(mapSkillEnd(sk))
	}
	return sk, nil
}

// Finish marks the AgentTask succeeded (or failed if err != nil) and emits agent_end/settled.
func (s *Scheduler) Finish(ctx context.Context, taskID string, data map[string]any, finishErr error) (*AgentTask, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	task.UpdatedAt = now
	task.EndedAt = &now
	if finishErr != nil {
		task.Status = StatusFailed
		task.Error = finishErr.Error()
		if data == nil {
			data = map[string]any{}
		}
		data["error"] = finishErr.Error()
	} else {
		task.Status = StatusSucceeded
	}
	if err := s.Store.SaveTask(ctx, task); err != nil {
		return nil, err
	}
	_ = s.checkpoint(ctx, task)
	s.emit(mapTaskEnd(taskID, data))
	s.emit(mapTaskSettled(taskID))
	return task, nil
}

// Pause checkpoints a running task for later Resume (断点).
func (s *Scheduler) Pause(ctx context.Context, taskID string) (*AgentTask, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status.Terminal() {
		return nil, fmt.Errorf("task %s is %s", taskID, task.Status)
	}
	now := time.Now().UTC()
	task.Status = StatusPaused
	task.UpdatedAt = now
	if err := s.Store.SaveTask(ctx, task); err != nil {
		return nil, err
	}
	_ = s.checkpoint(ctx, task)
	return task, nil
}

// Resume loads checkpoint and marks task running again (caller continues Turns).
func (s *Scheduler) Resume(ctx context.Context, taskID string) (*AgentTask, *Checkpoint, error) {
	task, err := s.Store.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if task.Status != StatusPaused && task.Status != StatusRunning {
		return nil, nil, fmt.Errorf("task %s cannot resume from %s", taskID, task.Status)
	}
	cp, err := s.Store.GetCheckpoint(ctx, taskID)
	if err != nil {
		cp = &Checkpoint{TaskID: taskID, CurrentTurnID: task.CurrentTurnID}
	}
	now := time.Now().UTC()
	task.Status = StatusRunning
	task.UpdatedAt = now
	if err := s.Store.SaveTask(ctx, task); err != nil {
		return nil, nil, err
	}
	s.emit(mapTaskStart(taskID)) // signal continuation
	return task, cp, nil
}

func (s *Scheduler) checkpoint(ctx context.Context, task *AgentTask) error {
	cp := &Checkpoint{
		TaskID:        task.ID,
		CurrentTurnID: task.CurrentTurnID,
		UpdatedAt:     time.Now().UTC(),
	}
	if prev, err := s.Store.GetCheckpoint(ctx, task.ID); err == nil && prev != nil {
		cp.Version = prev.Version + 1
	}
	return s.Store.SaveCheckpoint(ctx, cp)
}

// DefaultRoot returns DEER_FLOW_HOME-compatible task store root, or fallback.
func DefaultRoot(projectOrHome string) string {
	projectOrHome = strings.TrimSpace(projectOrHome)
	if projectOrHome == "" {
		return ".deer-flow"
	}
	return projectOrHome
}
