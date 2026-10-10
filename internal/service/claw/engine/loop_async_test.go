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

type seqProvider struct {
	n int
}

func (s *seqProvider) Model() string    { return "stub" }
func (s *seqProvider) Configured() bool { return true }

func (s *seqProvider) Chat(_ context.Context, req schema.ChatRequest) (*schema.ChatResponse, error) {
	s.n++
	if req.Tools == nil {
		return &schema.ChatResponse{
			Message: schema.Message{Role: schema.RoleAssistant, Content: "think"},
		}, nil
	}
	if s.n == 2 {
		return &schema.ChatResponse{
			Message: schema.Message{
				Role: schema.RoleAssistant,
				ToolCalls: []schema.ToolCall{{
					ID:        "c1",
					Name:      asyncToolPictureBook,
					Arguments: `{"prompt":"fox"}`,
				}},
			},
			FinishReason: "tool_calls",
		}, nil
	}
	return &schema.ChatResponse{
		Message:      schema.Message{Role: schema.RoleAssistant, Content: "方案：一只水彩狐狸"},
		FinishReason: "stop",
	}, nil
}

func TestAsyncPicturebookEmitsDeltaBeforeToolEnd(t *testing.T) {
	reg := tools.NewRegistry()
	var started atomic.Bool
	release := make(chan struct{})

	_ = reg.Register(schema.ToolDefinition{
		Name:        asyncToolPictureBook,
		Description: "draw",
		Parameters:  map[string]any{"type": "object"},
	}, func(ctx context.Context, arguments string) schema.ToolResult {
		started.Store(true)
		select {
		case <-release:
		case <-ctx.Done():
			return schema.ToolResult{Content: "cancelled", IsError: true}
		case <-time.After(5 * time.Second):
			return schema.ToolResult{Content: "timeout", IsError: true}
		}
		return schema.ToolResult{
			Content: "ok",
			Data:    map[string]any{"path": "/mnt/user-data/outputs/picturebook.png"},
		}
	})

	eng := New(&seqProvider{}, reg, Config{MaxTurns: 5, EnableThinking: true})
	var mu sync.Mutex
	var events []Event
	errCh := make(chan error, 1)
	go func() {
		_, err := eng.Run(context.Background(), []schema.Message{
			{Role: schema.RoleSystem, Content: "sys"},
			{Role: schema.RoleUser, Content: "画狐狸"},
		}, func(ev Event) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		})
		errCh <- err
	}()

	deadline := time.Now().Add(3 * time.Second)
	sawDelta := false
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, ev := range events {
			if ev.Type == "tool_end" {
				mu.Unlock()
				t.Fatalf("tool_end before release; events=%+v", events)
			}
			if ev.Type == "delta" && ev.Delta == "方案：一只水彩狐狸" {
				sawDelta = true
			}
		}
		mu.Unlock()
		if started.Load() && sawDelta {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !started.Load() {
		t.Fatal("async tool never started")
	}
	if !sawDelta {
		mu.Lock()
		t.Fatalf("expected prose delta before tool finished; events=%+v", events)
		mu.Unlock()
	}

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
	var sawEnd bool
	for _, ev := range events {
		if ev.Type == "tool_end" && ev.ToolName == asyncToolPictureBook && !ev.IsError {
			sawEnd = true
		}
	}
	if !sawEnd {
		t.Fatalf("missing successful tool_end; events=%+v", events)
	}
}
