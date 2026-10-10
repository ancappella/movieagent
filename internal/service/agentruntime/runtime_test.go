package agentruntime_test

import (
	"context"
	"path/filepath"
	"testing"

	agentctx "movieagent/internal/context"
	"movieagent/internal/service/agentruntime"
	"movieagent/internal/service/claw/schema"
	"movieagent/internal/service/task"
)

func TestRuntimeInferAndCheckpoint(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	rt, err := agentruntime.New(agentruntime.Options{
		Root:      root,
		SessionID: "chat",
		ThreadID:  "chat",
		TaskID:    "task-1",
		Model:     "test-model",
		WorkDir:   root,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskStore := task.NewFileStore(root)
	rt.Checkpoints.(*agentruntime.FileCheckpointer).WithTaskStore(taskStore)

	rt.Context.AppendUser("画一只狐狸")
	if err := rt.SeedInfer(ctx); err != nil {
		t.Fatal(err)
	}
	rt.SetTurn("turn-1")

	cp, err := rt.Barrier(ctx, agentruntime.PhaseCPThinking, agentruntime.Continuation{Kind: "need_action"}, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Version != 1 || cp.TurnID != "turn-1" {
		t.Fatalf("checkpoint = %+v", cp)
	}

	history := []schema.Message{
		{Role: schema.RoleSystem, Content: "sys"},
		{Role: schema.RoleUser, Content: "画一只狐狸"},
		{Role: schema.RoleAssistant, Content: "plan", ToolCalls: []schema.ToolCall{
			{ID: "c1", Name: "picturebook_illustrate", Arguments: `{"prompt":"fox"}`},
		}},
	}
	msgs := agentruntime.FromSchemaHistory(history)
	if err := rt.CaptureSchemaHistory(ctx, agentruntime.PhaseAction, msgs[2:], true); err != nil {
		t.Fatal(err)
	}

	pending := agentruntime.ToolStatesFromAssistant(history[2])
	if len(pending) != 1 || pending[0].Replay != "never" {
		t.Fatalf("pending = %+v", pending)
	}
	cp2, err := rt.Barrier(ctx, agentruntime.PhaseCPToolBatch, agentruntime.Continuation{Kind: "need_tools"}, 1, pending, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cp2.Version != 2 {
		t.Fatalf("version = %d", cp2.Version)
	}

	loaded, err := rt.Checkpoints.Load(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	plan := agentruntime.PlanResume(loaded)
	if plan.SkipDone || len(plan.Blocked) != 1 {
		t.Fatalf("plan = %+v", plan)
	}

	agentruntime.MarkToolCompleted(loaded, "c1", "ok", false)
	loaded.Phase = agentruntime.PhaseCPDone
	loaded.Continuation = agentruntime.Continuation{Kind: "may_finish"}
	if err := rt.Checkpoints.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	plan2 := agentruntime.PlanResume(loaded)
	if !plan2.SkipDone {
		t.Fatal("expected skip done")
	}

	// lightweight cursor synced
	light, err := taskStore.GetCheckpoint(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if light.CurrentTurnID != "turn-1" || light.Version < 2 {
		t.Fatalf("light = %+v", light)
	}

	snap, err := rt.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Infer) < 2 {
		t.Fatalf("infer records = %d", len(snap.Infer))
	}
	if _, err := filepath.Glob(filepath.Join(root, "infer", "task-1", "messages.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryUpsertAndInject(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	rt, err := agentruntime.New(agentruntime.Options{
		Root: root, SessionID: "s1", TaskID: "t1", WorkDir: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Memory.Upsert(ctx, agentruntime.MemoryItem{
		SessionID: "s1",
		Kind:      "preference",
		Content:   "喜欢水彩风格",
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.InjectMemory(ctx); err != nil {
		t.Fatal(err)
	}
	sys := rt.Context.Replay()[0]
	if sys.Kind != agentctx.KindSystem {
		t.Fatal("expected system")
	}
	if sys.Sections[agentctx.SectionExtra] == "" {
		t.Fatal("memory not injected")
	}
}
