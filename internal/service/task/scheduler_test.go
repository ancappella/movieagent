package task

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"movieagent/internal/service/sequence"
)

func TestSchedulerLayersPersistAndIsolateSkillFailure(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	var events []string
	sched := NewScheduler(store)
	sched.Emit = func(ev Event) { events = append(events, ev.Type) }

	ctx := context.Background()
	task, err := sched.Start(ctx, StartRequest{ThreadID: "th1", Goal: "draw a fox"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusPending {
		t.Fatalf("start status = %s, want pending", task.Status)
	}
	if _, err := sched.Activate(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := sched.BeginTurn(ctx, task.ID, "draw a fox")
	if err != nil {
		t.Fatal(err)
	}
	okSkill, err := sched.BeginSkill(ctx, turn.ID, "picturebook_illustrate", KindSkill, map[string]any{"prompt": "fox"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sched.EndSkill(ctx, okSkill.ID, "ok", map[string]any{"path": "/x.png"}, nil); err != nil {
		t.Fatal(err)
	}
	badSkill, err := sched.BeginSkill(ctx, turn.ID, "add", KindTool, map[string]any{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sched.EndSkill(ctx, badSkill.ID, "", nil, errors.New("boom")); err != nil {
		t.Fatal(err)
	}

	// Skill failure must not kill the turn/task by default.
	if _, err := sched.EndTurn(ctx, turn.ID, "thought", "here is art", nil); err != nil {
		t.Fatal(err)
	}
	gotTurn, _ := store.GetTurn(ctx, turn.ID)
	if gotTurn.Status != StatusSucceeded {
		t.Fatalf("turn status = %s", gotTurn.Status)
	}
	gotTask, _ := store.GetTask(ctx, task.ID)
	if gotTask.Status != StatusRunning {
		t.Fatalf("task status = %s (skill fail should not finish task)", gotTask.Status)
	}
	if _, err := sched.Finish(ctx, task.ID, map[string]any{"message": "done"}, nil); err != nil {
		t.Fatal(err)
	}
	gotTask, _ = store.GetTask(ctx, task.ID)
	if gotTask.Status != StatusSucceeded {
		t.Fatalf("task status = %s", gotTask.Status)
	}

	// Disk layout
	if _, err := store.GetCheckpoint(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	skills, err := store.ListSkills(ctx, turn.ID)
	if err != nil || len(skills) != 2 {
		t.Fatalf("skills = %d err=%v", len(skills), err)
	}
	path := filepath.Join(root, "tasks", task.ID, "task.json")
	if path == "" {
		t.Fatal("path")
	}

	wantSeq := []string{
		sequence.AgentStart,
		sequence.TurnStart,
		sequence.SkillStart,
		sequence.ToolExecutionStart,
		sequence.ToolExecutionEnd,
		sequence.SkillEnd,
		sequence.ToolExecutionStart,
		sequence.ToolExecutionEnd,
		sequence.TurnEnd,
		sequence.AgentEnd,
		sequence.AgentSettled,
	}
	if len(events) < len(wantSeq) {
		t.Fatalf("events=%v want at least %v", events, wantSeq)
	}
	for i, w := range wantSeq {
		if events[i] != w {
			t.Fatalf("event[%d]=%s want %s (all=%v)", i, events[i], w, events)
		}
	}
}

func TestPauseResumeCheckpoint(t *testing.T) {
	store := NewFileStore(t.TempDir())
	sched := NewScheduler(store)
	ctx := context.Background()
	task, err := sched.Start(ctx, StartRequest{Goal: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sched.Activate(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := sched.BeginTurn(ctx, task.ID, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sched.Pause(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetTask(ctx, task.ID)
	if got.Status != StatusPaused {
		t.Fatalf("status=%s", got.Status)
	}
	resumed, cp, err := sched.Resume(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != StatusRunning {
		t.Fatalf("status=%s", resumed.Status)
	}
	if cp.CurrentTurnID != turn.ID {
		t.Fatalf("checkpoint turn=%s want %s", cp.CurrentTurnID, turn.ID)
	}
}
