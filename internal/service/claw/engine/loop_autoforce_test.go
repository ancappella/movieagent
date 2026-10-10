package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"movieagent/internal/service/claw/schema"
	"movieagent/internal/service/claw/tools"
)

type skipToolProvider struct {
	n int
}

func (s *skipToolProvider) Model() string    { return "stub" }
func (s *skipToolProvider) Configured() bool { return true }

func (s *skipToolProvider) Chat(_ context.Context, req schema.ChatRequest) (*schema.ChatResponse, error) {
	s.n++
	if req.Tools == nil {
		return &schema.ChatResponse{
			Message: schema.Message{Role: schema.RoleAssistant, Content: "think draw sea"},
		}, nil
	}
	// Action: prose only — claims image is coming, never emits toolCalls.
	return &schema.ChatResponse{
		Message: schema.Message{
			Role:    schema.RoleAssistant,
			Content: "正在出图，稍后下方展示。\n\n橘色海水水彩插画。",
		},
		FinishReason: "stop",
	}, nil
}

func TestAutoPicturebookWhenModelSkipsTool(t *testing.T) {
	reg := tools.NewRegistry()
	var started atomic.Bool
	release := make(chan struct{})

	_ = reg.Register(schema.ToolDefinition{
		Name:        asyncToolPictureBook,
		Description: "draw",
		Parameters:  map[string]any{"type": "object"},
	}, func(ctx context.Context, arguments string) schema.ToolResult {
		started.Store(true)
		if arguments == "" || arguments == "{}" {
			return schema.ToolResult{Content: "empty args", IsError: true}
		}
		select {
		case <-release:
		case <-ctx.Done():
			return schema.ToolResult{Content: "cancelled", IsError: true}
		case <-time.After(5 * time.Second):
			return schema.ToolResult{Content: "timeout", IsError: true}
		}
		return schema.ToolResult{
			Content: "ok",
			Data:    map[string]any{"path": "/mnt/user-data/outputs/picturebook.png", "width": 512, "height": 768},
		}
	})

	eng := New(&skipToolProvider{}, reg, Config{MaxTurns: 3, EnableThinking: true})
	var mu sync.Mutex
	var events []Event
	errCh := make(chan error, 1)
	go func() {
		_, err := eng.Run(context.Background(), []schema.Message{
			{Role: schema.RoleSystem, Content: "sys"},
			{Role: schema.RoleUser, Content: "绘制橘色的海水"},
		}, func(ev Event) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		})
		errCh <- err
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if started.Load() {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !started.Load() {
		mu.Lock()
		t.Fatalf("expected auto picturebook start; events=%+v", events)
		mu.Unlock()
	}

	// Done must not appear before the async tool finishes.
	mu.Lock()
	for _, ev := range events {
		if ev.Type == "done" {
			mu.Unlock()
			t.Fatalf("done before image tool finished; events=%+v", events)
		}
	}
	mu.Unlock()

	close(release)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not finish")
	}

	mu.Lock()
	defer mu.Unlock()
	var sawStart, sawEnd, sawDone, sawDelta bool
	for _, ev := range events {
		switch {
		case ev.Type == "tool_start" && ev.ToolName == asyncToolPictureBook:
			sawStart = true
		case ev.Type == "tool_end" && ev.ToolName == asyncToolPictureBook && !ev.IsError:
			sawEnd = true
		case ev.Type == "done":
			sawDone = true
		case ev.Type == "delta":
			sawDelta = true
		}
	}
	if !sawDelta || !sawStart || !sawEnd || !sawDone {
		t.Fatalf("missing events delta=%v start=%v end=%v done=%v; events=%+v",
			sawDelta, sawStart, sawEnd, sawDone, events)
	}
}

func TestWantsIllustrationHeuristics(t *testing.T) {
	if !wantsIllustration("绘制橘色的海水", "") {
		t.Fatal("user draw intent")
	}
	if !wantsIllustration("随便聊聊", "正在出图，插画稍后展示") {
		t.Fatal("assistant claim")
	}
	if wantsIllustration("把这张图做成动画", "好的") {
		t.Fatal("pure animate should not force illustrate")
	}
	if !wantsAnimation("把这张图做成动画", "") {
		t.Fatal("animate intent")
	}
	u := "用上述的图 生成5s的动画"
	a := "正在出动画，5 秒成片稍后展示 🎬"
	if !wantsAnimation(u, a) {
		t.Fatal("5s animation intent")
	}
	if wantsIllustration(u, a) {
		t.Fatal("animation request must not force illustrate")
	}
	if parseDurationSec(u, 4) != 5 {
		t.Fatalf("duration=%v want 5", parseDurationSec(u, 4))
	}
}
