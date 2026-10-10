package chathub

import (
	"testing"
	"time"
)

func TestSubscribeReplaysHistory(t *testing.T) {
	h := New()
	h.Publish("t1", Event{Type: "thinking_delta", Delta: "a"})
	h.Publish("t1", Event{Type: "delta", Delta: "b"})

	hist, live, unsub := h.Subscribe("t1")
	defer unsub()
	if len(hist) != 2 {
		t.Fatalf("history=%d", len(hist))
	}
	h.Publish("t1", Event{Type: "done", Model: "m"})
	select {
	case ev := <-live:
		if ev.Type != "done" {
			t.Fatalf("got %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for done")
	}
}
