package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentctx "movieagent/internal/context"
)

// Infer phases for transcript records.
const (
	PhaseSeed     = "seed"
	PhaseThinking = "thinking"
	PhaseAction   = "action"
	PhaseTool     = "tool"
	PhaseFinal    = "final"
)

// InferScope selects which transcript partition to read/write.
type InferScope struct {
	SessionID string
	TaskID    string
	TurnID    string
}

// InferRecord is one persisted inference message.
type InferRecord struct {
	Seq       int64            `json:"seq"`
	SessionID string           `json:"session_id,omitempty"`
	TaskID    string           `json:"task_id,omitempty"`
	TurnID    string           `json:"turn_id,omitempty"`
	Phase     string           `json:"phase,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	Message   agentctx.Message `json:"message"`
}

// InferStore persists ordered inference messages for a task.
type InferStore interface {
	Append(ctx context.Context, scope InferScope, phase string, msg agentctx.Message) (*InferRecord, error)
	AppendBatch(ctx context.Context, scope InferScope, phase string, msgs []agentctx.Message) error
	List(ctx context.Context, scope InferScope) ([]InferRecord, error)
	Clear(ctx context.Context, scope InferScope) error
}

// FileInferStore writes JSONL under {root}/infer/{taskID}/messages.jsonl.
type FileInferStore struct {
	root string
	mu   sync.Mutex
}

func NewFileInferStore(root string) *FileInferStore {
	return &FileInferStore{root: strings.TrimSpace(root)}
}

func (s *FileInferStore) dir(taskID string) string {
	return filepath.Join(s.root, "infer", strings.TrimSpace(taskID))
}

func (s *FileInferStore) path(taskID string) string {
	return filepath.Join(s.dir(taskID), "messages.jsonl")
}

func (s *FileInferStore) Append(_ context.Context, scope InferScope, phase string, msg agentctx.Message) (*InferRecord, error) {
	taskID := strings.TrimSpace(scope.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("infer: task_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	seq, err := s.nextSeqLocked(taskID)
	if err != nil {
		return nil, err
	}
	rec := InferRecord{
		Seq:       seq,
		SessionID: strings.TrimSpace(scope.SessionID),
		TaskID:    taskID,
		TurnID:    strings.TrimSpace(scope.TurnID),
		Phase:     strings.TrimSpace(phase),
		CreatedAt: time.Now().UTC(),
		Message:   cloneMessage(msg),
	}
	if rec.Message.CreatedAt.IsZero() {
		rec.Message.CreatedAt = rec.CreatedAt
	}
	if err := s.appendLocked(taskID, rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *FileInferStore) AppendBatch(ctx context.Context, scope InferScope, phase string, msgs []agentctx.Message) error {
	for _, m := range msgs {
		if _, err := s.Append(ctx, scope, phase, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileInferStore) List(_ context.Context, scope InferScope) ([]InferRecord, error) {
	taskID := strings.TrimSpace(scope.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("infer: task_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	wantTurn := strings.TrimSpace(scope.TurnID)
	wantSess := strings.TrimSpace(scope.SessionID)
	var out []InferRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec InferRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if wantTurn != "" && rec.TurnID != wantTurn {
			continue
		}
		if wantSess != "" && rec.SessionID != "" && rec.SessionID != wantSess {
			continue
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

func (s *FileInferStore) Clear(_ context.Context, scope InferScope) error {
	taskID := strings.TrimSpace(scope.TaskID)
	if taskID == "" {
		return fmt.Errorf("infer: task_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path(taskID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *FileInferStore) nextSeqLocked(taskID string) (int64, error) {
	f, err := os.Open(s.path(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}
	defer f.Close()
	var last int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec InferRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec.Seq > last {
			last = rec.Seq
		}
	}
	return last + 1, sc.Err()
}

func (s *FileInferStore) appendLocked(taskID string, rec InferRecord) error {
	if err := os.MkdirAll(s.dir(taskID), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path(taskID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}
