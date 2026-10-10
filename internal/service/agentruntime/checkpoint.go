package agentruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentctx "movieagent/internal/context"
	"movieagent/internal/service/task"
)

// Phase is the recovery cursor inside a turn (pi-agent style).
type Phase string

const (
	PhaseCPThinking   Phase = "thinking"
	PhaseCPAction     Phase = "action"
	PhaseCPToolBatch  Phase = "tool_batch"
	PhaseCPAwaitAsync Phase = "await_async"
	PhaseCPDone       Phase = "done"
)

// Continuation tells Resume what to do next.
type Continuation struct {
	Kind string `json:"kind"` // need_action | need_tools | may_finish | await_async
}

// ToolCallState tracks one tool invocation for safe replay.
type ToolCallState struct {
	ToolCallID string         `json:"tool_call_id"`
	Name       string         `json:"name"`
	Arguments  string         `json:"arguments,omitempty"`
	Status     string         `json:"status"` // planned | running | completed | failed
	Replay     string         `json:"replay"` // safe | never
	Result     string         `json:"result,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
	Memo       map[string]any `json:"memo,omitempty"`
}

// AgentCheckpoint is the durable recovery snapshot for one AgentTask.
type AgentCheckpoint struct {
	Version      int                 `json:"version"`
	TaskID       string              `json:"task_id"`
	SessionID    string              `json:"session_id,omitempty"`
	ThreadID     string              `json:"thread_id,omitempty"`
	TurnID       string              `json:"turn_id,omitempty"`
	Model        string              `json:"model,omitempty"`
	Phase        Phase               `json:"phase"`
	Continuation Continuation        `json:"continuation"`
	EngineStep   int                 `json:"engine_step,omitempty"`
	TriggerMsgID string              `json:"trigger_msg_id,omitempty"`
	Messages     []agentctx.Message  `json:"messages,omitempty"`
	PendingTools []ToolCallState     `json:"pending_tools,omitempty"`
	ArtifactIDs  []string            `json:"artifact_ids,omitempty"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// Checkpointer persists full AgentCheckpoint documents.
type Checkpointer interface {
	Save(ctx context.Context, cp *AgentCheckpoint) error
	Load(ctx context.Context, taskID string) (*AgentCheckpoint, error)
	Delete(ctx context.Context, taskID string) error
}

// FileCheckpointer stores checkpoint.full.json under the task directory
// and keeps task.Checkpoint (lightweight cursor) in sync when a task.Store is provided.
type FileCheckpointer struct {
	root      string
	taskStore task.Store // optional
	mu        sync.Mutex
}

func NewFileCheckpointer(root string) *FileCheckpointer {
	return &FileCheckpointer{root: strings.TrimSpace(root)}
}

// WithTaskStore syncs the lightweight task.Checkpoint cursor on Save.
func (c *FileCheckpointer) WithTaskStore(store task.Store) *FileCheckpointer {
	c.taskStore = store
	return c
}

func (c *FileCheckpointer) fullPath(taskID string) string {
	return filepath.Join(c.root, "tasks", strings.TrimSpace(taskID), "checkpoint.full.json")
}

func (c *FileCheckpointer) Save(ctx context.Context, cp *AgentCheckpoint) error {
	if cp == nil || strings.TrimSpace(cp.TaskID) == "" {
		return fmt.Errorf("checkpoint: empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	prev, err := c.loadLocked(cp.TaskID)
	if err == nil && prev != nil {
		cp.Version = prev.Version + 1
	} else if cp.Version == 0 {
		cp.Version = 1
	}
	cp.UpdatedAt = time.Now().UTC()
	cp.Messages = cloneMessages(cp.Messages)
	cp.PendingTools = append([]ToolCallState(nil), cp.PendingTools...)
	cp.ArtifactIDs = append([]string(nil), cp.ArtifactIDs...)

	if err := writeJSON(c.fullPath(cp.TaskID), cp); err != nil {
		return err
	}

	// Keep lightweight task cursor in sync when available.
	if c.taskStore != nil {
		light := &task.Checkpoint{
			TaskID:        cp.TaskID,
			CurrentTurnID: cp.TurnID,
			Version:       cp.Version,
			UpdatedAt:     cp.UpdatedAt,
		}
		_ = c.taskStore.SaveCheckpoint(ctx, light)
	}
	return nil
}

func (c *FileCheckpointer) Load(_ context.Context, taskID string) (*AgentCheckpoint, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("checkpoint: task_id required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadLocked(taskID)
}

func (c *FileCheckpointer) loadLocked(taskID string) (*AgentCheckpoint, error) {
	var cp AgentCheckpoint
	if err := readJSON(c.fullPath(taskID), &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

func (c *FileCheckpointer) Delete(_ context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("checkpoint: task_id required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.Remove(c.fullPath(taskID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ResumePlan describes how a worker should continue after Load.
type ResumePlan struct {
	Checkpoint *AgentCheckpoint
	SkipDone   bool
	ReplaySafe []ToolCallState // tools still needing execution with replay=safe
	Blocked    []ToolCallState // replay=never and not completed
}

// PlanResume inspects a checkpoint and returns what Resume should do.
func PlanResume(cp *AgentCheckpoint) ResumePlan {
	plan := ResumePlan{Checkpoint: cp}
	if cp == nil {
		return plan
	}
	if cp.Phase == PhaseCPDone || cp.Continuation.Kind == "may_finish" {
		plan.SkipDone = true
		return plan
	}
	for _, t := range cp.PendingTools {
		switch t.Status {
		case "completed", "failed":
			continue
		default:
			if t.Replay == "never" {
				plan.Blocked = append(plan.Blocked, t)
			} else {
				plan.ReplaySafe = append(plan.ReplaySafe, t)
			}
		}
	}
	return plan
}

// MarkToolCompleted updates a pending tool in-place by id.
func MarkToolCompleted(cp *AgentCheckpoint, toolCallID, result string, isError bool) {
	if cp == nil {
		return
	}
	for i := range cp.PendingTools {
		if cp.PendingTools[i].ToolCallID == toolCallID {
			cp.PendingTools[i].Status = "completed"
			if isError {
				cp.PendingTools[i].Status = "failed"
			}
			cp.PendingTools[i].Result = result
			cp.PendingTools[i].IsError = isError
			return
		}
	}
}

// DefaultReplayForTool chooses replay policy (async media tools are never auto-replayed).
func DefaultReplayForTool(name string) string {
	switch strings.TrimSpace(name) {
	case "picturebook_illustrate", "picturebook_animate":
		return "never"
	default:
		return "safe"
	}
}
