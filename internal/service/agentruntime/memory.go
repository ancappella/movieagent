package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MemoryItem is one long-lived fact for a session.
type MemoryItem struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      string    `json:"kind,omitempty"` // fact | preference | summary
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// MemoryStore persists cross-turn session memory.
type MemoryStore interface {
	List(ctx context.Context, sessionID string) ([]MemoryItem, error)
	Upsert(ctx context.Context, item MemoryItem) error
	Delete(ctx context.Context, sessionID, id string) error
}

// FileMemoryStore keeps memory at {root}/sessions/{sessionID}/memory.json.
type FileMemoryStore struct {
	root string
	mu   sync.Mutex
}

func NewFileMemoryStore(root string) *FileMemoryStore {
	return &FileMemoryStore{root: strings.TrimSpace(root)}
}

func (s *FileMemoryStore) path(sessionID string) string {
	return filepath.Join(s.root, "sessions", strings.TrimSpace(sessionID), "memory.json")
}

type memoryFile struct {
	Items []MemoryItem `json:"items"`
}

func (s *FileMemoryStore) List(_ context.Context, sessionID string) ([]MemoryItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("memory: session_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var f memoryFile
	if err := readJSON(s.path(sessionID), &f); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return append([]MemoryItem(nil), f.Items...), nil
}

func (s *FileMemoryStore) Upsert(_ context.Context, item MemoryItem) error {
	item.SessionID = strings.TrimSpace(item.SessionID)
	item.Content = strings.TrimSpace(item.Content)
	if item.SessionID == "" || item.Content == "" {
		return fmt.Errorf("memory: session_id and content required")
	}
	if item.ID == "" {
		item.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()

	var f memoryFile
	path := s.path(item.SessionID)
	if err := readJSON(path, &f); err != nil && !os.IsNotExist(err) {
		return err
	}
	found := false
	for i := range f.Items {
		if f.Items[i].ID == item.ID {
			f.Items[i] = item
			found = true
			break
		}
	}
	if !found {
		f.Items = append(f.Items, item)
	}
	return writeJSON(path, f)
}

func (s *FileMemoryStore) Delete(_ context.Context, sessionID, id string) error {
	sessionID = strings.TrimSpace(sessionID)
	id = strings.TrimSpace(id)
	if sessionID == "" || id == "" {
		return fmt.Errorf("memory: session_id and id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.path(sessionID)
	var f memoryFile
	if err := readJSON(path, &f); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	out := f.Items[:0]
	for _, it := range f.Items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	f.Items = out
	return writeJSON(path, f)
}

func readJSON(path string, dest any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
