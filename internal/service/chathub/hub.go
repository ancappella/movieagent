// Package chathub fans out chat task progress events to SSE subscribers.
package chathub

import (
	"sync"
)

// Event is one progress update for a chat task (SSE payload).
type Event struct {
	Type    string `json:"type"` // task | progress | thinking_delta | status | delta | image | done | error
	TaskID  string `json:"task_id,omitempty"`
	Status  string `json:"status,omitempty"`
	Delta   string `json:"delta,omitempty"`
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
	Model   string `json:"model,omitempty"`
	Message string `json:"message,omitempty"`
}

const subBuffer = 64

type subscriber struct {
	ch   chan Event
	once sync.Once
}

func (s *subscriber) close() {
	s.once.Do(func() { close(s.ch) })
}

// Hub multiplexes events per task_id for late-joining SSE clients.
type Hub struct {
	mu      sync.Mutex
	subs    map[string]map[*subscriber]struct{}
	history map[string][]Event
	closed  map[string]bool
}

// New creates an empty Hub.
func New() *Hub {
	return &Hub{
		subs:    make(map[string]map[*subscriber]struct{}),
		history: make(map[string][]Event),
		closed:  make(map[string]bool),
	}
}

// Publish appends to history and fans out to live subscribers.
func (h *Hub) Publish(taskID string, ev Event) {
	if h == nil || taskID == "" {
		return
	}
	ev.TaskID = taskID
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history[taskID] = append(h.history[taskID], ev)
	for sub := range h.subs[taskID] {
		select {
		case sub.ch <- ev:
		default:
			// Drop if subscriber is slow; history still retained for reconnect.
		}
	}
	if ev.Type == "done" || ev.Type == "error" {
		h.closed[taskID] = true
		for sub := range h.subs[taskID] {
			sub.close()
		}
		delete(h.subs, taskID)
	}
}

// Subscribe returns a snapshot of past events and a live channel for new ones.
// Caller should write history first, then read live until closed.
// unsub must be called to release the subscription if the client disconnects early.
func (h *Hub) Subscribe(taskID string) (history []Event, live <-chan Event, unsub func()) {
	sub := &subscriber{ch: make(chan Event, subBuffer)}
	noop := func() {}
	if h == nil || taskID == "" {
		sub.close()
		return nil, sub.ch, noop
	}
	h.mu.Lock()
	history = append([]Event(nil), h.history[taskID]...)
	alreadyClosed := h.closed[taskID]
	if alreadyClosed {
		h.mu.Unlock()
		sub.close()
		return history, sub.ch, noop
	}
	if h.subs[taskID] == nil {
		h.subs[taskID] = make(map[*subscriber]struct{})
	}
	h.subs[taskID][sub] = struct{}{}
	h.mu.Unlock()

	unsub = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if set, ok := h.subs[taskID]; ok {
			if _, exists := set[sub]; exists {
				delete(set, sub)
				sub.close()
			}
			if len(set) == 0 {
				delete(h.subs, taskID)
			}
		}
	}
	return history, sub.ch, unsub
}

// Has reports whether any events exist for taskID.
func (h *Hub) Has(taskID string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.history[taskID]
	return ok || h.closed[taskID]
}
