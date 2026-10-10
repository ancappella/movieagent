package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileStore persists each layer as JSON under {root}/tasks/{taskID}/.
//
//	task.json
//	checkpoint.json
//	turns/{turnID}.json
//	skills/{skillRunID}.json
type FileStore struct {
	root string
	mu   sync.Mutex
}

// NewFileStore creates a store rooted at root (usually DEER_FLOW_HOME/tasks parent).
// Files land in root/tasks/<id>/...
func NewFileStore(root string) *FileStore {
	return &FileStore{root: strings.TrimSpace(root)}
}

func (s *FileStore) taskDir(taskID string) string {
	return filepath.Join(s.root, "tasks", taskID)
}

func (s *FileStore) writeJSON(path string, v any) error {
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

func (s *FileStore) readJSON(path string, dest any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

func (s *FileStore) SaveTask(_ context.Context, t *AgentTask) error {
	if t == nil || t.ID == "" {
		return fmt.Errorf("task: empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON(filepath.Join(s.taskDir(t.ID), "task.json"), t)
}

func (s *FileStore) GetTask(_ context.Context, id string) (*AgentTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var t AgentTask
	if err := s.readJSON(filepath.Join(s.taskDir(id), "task.json"), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *FileStore) ListTasks(_ context.Context, threadID string) ([]*AgentTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.root, "tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*AgentTask
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var t AgentTask
		if err := s.readJSON(filepath.Join(root, e.Name(), "task.json"), &t); err != nil {
			continue
		}
		if threadID != "" && t.ThreadID != threadID {
			continue
		}
		cp := t
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *FileStore) SaveTurn(_ context.Context, t *Turn) error {
	if t == nil || t.ID == "" || t.TaskID == "" {
		return fmt.Errorf("turn: empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON(filepath.Join(s.taskDir(t.TaskID), "turns", t.ID+".json"), t)
}

func (s *FileStore) GetTurn(_ context.Context, id string) (*Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Scan tasks/*/turns/id.json — ids are unique.
	root := filepath.Join(s.root, "tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), "turns", id+".json")
		var t Turn
		if err := s.readJSON(path, &t); err == nil {
			return &t, nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *FileStore) ListTurns(_ context.Context, taskID string) ([]*Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.taskDir(taskID), "turns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Turn
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var t Turn
		if err := s.readJSON(filepath.Join(dir, e.Name()), &t); err != nil {
			continue
		}
		cp := t
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func (s *FileStore) SaveSkill(_ context.Context, sk *SkillRun) error {
	if sk == nil || sk.ID == "" || sk.TaskID == "" {
		return fmt.Errorf("skill: empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON(filepath.Join(s.taskDir(sk.TaskID), "skills", sk.ID+".json"), sk)
}

func (s *FileStore) GetSkill(_ context.Context, id string) (*SkillRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.root, "tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), "skills", id+".json")
		var sk SkillRun
		if err := s.readJSON(path, &sk); err == nil {
			return &sk, nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *FileStore) ListSkills(_ context.Context, turnID string) ([]*SkillRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, err := s.getTurnUnlocked(turnID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.taskDir(turn.TaskID), "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*SkillRun
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var sk SkillRun
		if err := s.readJSON(filepath.Join(dir, e.Name()), &sk); err != nil {
			continue
		}
		if sk.TurnID != turnID {
			continue
		}
		cp := sk
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *FileStore) getTurnUnlocked(id string) (*Turn, error) {
	root := filepath.Join(s.root, "tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), "turns", id+".json")
		var t Turn
		if err := s.readJSON(path, &t); err == nil {
			return &t, nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *FileStore) SaveCheckpoint(_ context.Context, cp *Checkpoint) error {
	if cp == nil || cp.TaskID == "" {
		return fmt.Errorf("checkpoint: empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON(filepath.Join(s.taskDir(cp.TaskID), "checkpoint.json"), cp)
}

func (s *FileStore) GetCheckpoint(_ context.Context, taskID string) (*Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cp Checkpoint
	if err := s.readJSON(filepath.Join(s.taskDir(taskID), "checkpoint.json"), &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}
