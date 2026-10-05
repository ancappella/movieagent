package herness

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type scriptedModel struct {
	calls int
	script []ModelResponse
}

func (m *scriptedModel) Chat(_ context.Context, _ ModelRequest) (ModelResponse, error) {
	if m.calls >= len(m.script) {
		return ModelResponse{Message: NewAssistantMessage("done")}, nil
	}
	resp := m.script[m.calls]
	m.calls++
	return resp, nil
}

func TestMainLoopEchoTool(t *testing.T) {
	model := &scriptedModel{script: []ModelResponse{
		{Message: NewAssistantMessage("", ToolCall{
			ID:   "c1",
			Name: "echo",
			Args: mustJSON(map[string]string{"text": "hi"}),
		})},
		{Message: NewAssistantMessage("echoed: hi")},
	}}
	agent, err := NewAgent(Options{
		Model: model,
		Features: &Features{BuiltinTools: true, LoopDetection: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(agent)
	got, err := client.Chat(context.Background(), "say hi via echo", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "echoed: hi" {
		t.Fatalf("response = %q", got)
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d", model.calls)
	}
}

func TestStreamEmitsEnd(t *testing.T) {
	model := ModelFunc(func(_ context.Context, _ ModelRequest) (ModelResponse, error) {
		return ModelResponse{Message: NewAssistantMessage("ok")}, nil
	})
	agent, err := NewAgent(Options{Model: model, Features: &Features{BuiltinTools: false}})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewClient(agent).Stream(context.Background(), "hi", "t-stream")
	if err != nil {
		t.Fatal(err)
	}
	var types []StreamEventType
	for ev := range ch {
		types = append(types, ev.Type)
	}
	if len(types) == 0 || types[len(types)-1] != EventEnd {
		t.Fatalf("events = %v", types)
	}
}

func TestCheckpointerKeepsHistory(t *testing.T) {
	cp := NewMemoryCheckpointer()
	n := 0
	model := ModelFunc(func(_ context.Context, req ModelRequest) (ModelResponse, error) {
		n++
		userCount := 0
		for _, m := range req.Messages {
			if m.Role == RoleUser {
				userCount++
			}
		}
		return ModelResponse{Message: NewAssistantMessage(strings.Repeat("x", userCount))}, nil
	})
	agent, err := NewAgent(Options{
		Model:        model,
		Checkpointer: cp,
		Features:     &Features{BuiltinTools: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(agent)
	a, err := client.Chat(context.Background(), "one", "hist")
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.Chat(context.Background(), "two", "hist")
	if err != nil {
		t.Fatal(err)
	}
	if a != "x" || b != "xx" {
		t.Fatalf("a=%q b=%q", a, b)
	}
}

func TestDanglingToolCallFilled(t *testing.T) {
	state := ThreadState{Messages: []Message{
		NewAssistantMessage("working", ToolCall{ID: "orphan", Name: "echo", Args: json.RawMessage(`{"text":"x"}`)}),
	}}
	mw := DanglingToolCallMiddleware{}
	if err := mw.BeforeAgent(context.Background(), &state, &RunConfig{}); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 2 || state.Messages[1].Role != RoleTool {
		t.Fatalf("messages = %#v", state.Messages)
	}
}

func TestSandboxMiddlewareAcquire(t *testing.T) {
	state := ThreadState{}
	mw := SandboxMiddleware{Acquire: func(context.Context, *RunConfig) (string, error) {
		return "local:u:t", nil
	}}
	if err := mw.BeforeAgent(context.Background(), &state, &RunConfig{UserID: "u", ThreadID: "t"}); err != nil {
		t.Fatal(err)
	}
	if state.SandboxID != "local:u:t" {
		t.Fatalf("sandbox = %q", state.SandboxID)
	}
}
