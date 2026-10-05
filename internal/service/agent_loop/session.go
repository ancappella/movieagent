package agent_loop

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Session is one robot conversation: durable transcript + last workflow payload.
type Session struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	SkillID   string         `json:"skill_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Messages  []Message      `json:"messages"`
	LastData  map[string]any `json:"last_data,omitempty"`

	mu     sync.Mutex
	closed bool
}

func (s *Session) Snapshot() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *s
	out.Messages = append([]Message(nil), s.Messages...)
	if s.LastData != nil {
		out.LastData = copyMap(s.LastData)
	}
	return out
}

func (s *Session) append(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now().UTC()
}

func (s *Session) SetSkill(skillID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SkillID = strings.TrimSpace(skillID)
	s.UpdatedAt = time.Now().UTC()
}

func (s *Session) setLast(data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastData = copyMap(data)
	s.UpdatedAt = time.Now().UTC()
}

func (s *Session) messagesCopy() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.Messages...)
}

func copyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Hub stores in-memory robot sessions for the process lifetime.
type Hub struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewHub() *Hub {
	return &Hub{sessions: map[string]*Session{}}
}

func NewSessionID() string {
	return fmt.Sprintf("rs-%d", time.Now().UnixNano())
}

func (h *Hub) Open(agentID, skillID string) *Session {
	if agentID == "" {
		agentID = "brand-agent"
	}
	now := time.Now().UTC()
	s := &Session{
		ID:        NewSessionID(),
		AgentID:   agentID,
		SkillID:   strings.TrimSpace(skillID),
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.mu.Lock()
	h.sessions[s.ID] = s
	h.mu.Unlock()
	return s
}

func (h *Hub) Get(id string) (*Session, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	return s, ok
}
