package agent_loop

import (
	"context"
	"testing"
)

func TestRunPromptEventOrder(t *testing.T) {
	h := NewHub()
	sess := h.Open("brand-agent", "poster-workflow")
	var types []string
	err := RunPrompt(context.Background(), sess, "hello", func(ev Event) {
		types = append(types, ev.Type)
	}, func(context.Context, *Session, string, Emit) (map[string]any, error) {
		return map[string]any{"message": "ok"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agent_start", "turn_start",
		"message_start", "message_end",
		"thinking_start", "thinking_end",
		"message_start", "message_update", "message_end",
		"turn_end", "agent_end", "agent_settled",
	}
	if len(types) != len(want) {
		t.Fatalf("events %v", types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("event[%d]=%s want %s", i, types[i], want[i])
		}
	}
	if got := sess.messagesCopy(); len(got) != 2 {
		t.Fatalf("messages=%d", len(got))
	}
}
