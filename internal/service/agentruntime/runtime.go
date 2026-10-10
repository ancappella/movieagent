// Package agentruntime owns the durable agent execution facade:
// identity (session/task/turn), context, memory, inference transcript, and checkpoints.
package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	agentctx "movieagent/internal/context"
)

// Identity keys one agent run.
type Identity struct {
	SessionID string `json:"session_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
}

// Snapshot is a read-only view of runtime state for debugging / resume planning.
type Snapshot struct {
	Identity Identity             `json:"identity"`
	Model    string               `json:"model,omitempty"`
	Context  []agentctx.Message   `json:"context,omitempty"`
	Memory   []MemoryItem         `json:"memory,omitempty"`
	Infer    []InferRecord        `json:"infer,omitempty"`
	Checkpoint *AgentCheckpoint   `json:"checkpoint,omitempty"`
}

// Options configure a Runtime.
type Options struct {
	Root      string // RuntimeHome / DEER_FLOW_HOME
	SessionID string
	ThreadID  string
	TaskID    string
	Model     string
	WorkDir   string
}

// Runtime is the session-facing agent state bag used by chat / claw.
type Runtime struct {
	mu sync.RWMutex

	ID      Identity
	Model   string
	Context *agentctx.Session

	Infer      InferStore
	Memory     MemoryStore
	Checkpoints Checkpointer
}

// New builds a Runtime rooted at opts.Root.
func New(opts Options) (*Runtime, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return nil, fmt.Errorf("agentruntime: root required")
	}
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		sessionID = "chat"
	}
	threadID := strings.TrimSpace(opts.ThreadID)
	if threadID == "" {
		threadID = sessionID
	}
	taskID := strings.TrimSpace(opts.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("agentruntime: task_id required")
	}

	rt := &Runtime{
		ID: Identity{
			SessionID: sessionID,
			ThreadID:  threadID,
			TaskID:    taskID,
		},
		Model:       strings.TrimSpace(opts.Model),
		Context:     agentctx.NewSession(sessionID, opts.WorkDir),
		Infer:       NewFileInferStore(root),
		Memory:      NewFileMemoryStore(root),
		Checkpoints: NewFileCheckpointer(root),
	}
	return rt, nil
}

// SetTurn updates the current turn id.
func (r *Runtime) SetTurn(turnID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ID.TurnID = strings.TrimSpace(turnID)
}

// SetModel records the active model id.
func (r *Runtime) SetModel(model string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Model = strings.TrimSpace(model)
}

// IdentityCopy returns a snapshot of identity fields.
func (r *Runtime) IdentityCopy() Identity {
	if r == nil {
		return Identity{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ID
}

// InjectMemory loads session memory into the system section (best-effort).
func (r *Runtime) InjectMemory(ctx context.Context) error {
	if r == nil || r.Memory == nil || r.Context == nil {
		return nil
	}
	items, err := r.Memory.List(ctx, r.ID.SessionID)
	if err != nil || len(items) == 0 {
		return err
	}
	var b strings.Builder
	b.WriteString("### 会话记忆\n")
	for i, it := range items {
		if i >= 20 {
			break
		}
		line := strings.TrimSpace(it.Content)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		if it.Kind != "" {
			b.WriteString("[")
			b.WriteString(it.Kind)
			b.WriteString("] ")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	r.Context.SetSection(agentctx.SectionExtra, strings.TrimSpace(b.String()))
	return nil
}

// SeedInfer persists the current context as the seed transcript for this task.
func (r *Runtime) SeedInfer(ctx context.Context) error {
	if r == nil || r.Infer == nil || r.Context == nil {
		return nil
	}
	id := r.IdentityCopy()
	return r.Infer.AppendBatch(ctx, InferScope{
		SessionID: id.SessionID,
		TaskID:    id.TaskID,
		TurnID:    id.TurnID,
	}, PhaseSeed, r.Context.Replay())
}

// CaptureSchemaHistory appends engine/provider history into the infer store and
// optionally syncs assistant/tool messages back into Context.
func (r *Runtime) CaptureSchemaHistory(ctx context.Context, phase string, msgs []agentctx.Message, syncContext bool) error {
	if r == nil || len(msgs) == 0 {
		return nil
	}
	id := r.IdentityCopy()
	if r.Infer != nil {
		if err := r.Infer.AppendBatch(ctx, InferScope{
			SessionID: id.SessionID,
			TaskID:    id.TaskID,
			TurnID:    id.TurnID,
		}, phase, msgs); err != nil {
			return err
		}
	}
	if syncContext && r.Context != nil {
		for _, m := range msgs {
			switch m.Kind {
			case agentctx.KindAssistant:
				r.Context.AppendAssistant(m.Content, m.Thinking, m.ToolCalls...)
			case agentctx.KindToolResult:
				r.Context.AppendToolResult(m.ToolCallID, m.ToolName, m.Content, m.IsError)
			case agentctx.KindUser:
				// user already seeded; skip duplicates
			}
		}
	}
	return nil
}

// Snapshot builds a read-only view.
func (r *Runtime) Snapshot(ctx context.Context) (Snapshot, error) {
	out := Snapshot{}
	if r == nil {
		return out, fmt.Errorf("agentruntime: nil")
	}
	id := r.IdentityCopy()
	out.Identity = id
	r.mu.RLock()
	out.Model = r.Model
	r.mu.RUnlock()
	if r.Context != nil {
		out.Context = r.Context.Replay()
	}
	if r.Memory != nil {
		mem, err := r.Memory.List(ctx, id.SessionID)
		if err != nil {
			return out, err
		}
		out.Memory = mem
	}
	if r.Infer != nil {
		recs, err := r.Infer.List(ctx, InferScope{TaskID: id.TaskID, SessionID: id.SessionID})
		if err != nil {
			return out, err
		}
		out.Infer = recs
	}
	if r.Checkpoints != nil {
		cp, err := r.Checkpoints.Load(ctx, id.TaskID)
		if err == nil {
			out.Checkpoint = cp
		}
	}
	return out, nil
}

// Barrier writes a durable AgentCheckpoint (pi-agent style recovery cursor).
func (r *Runtime) Barrier(ctx context.Context, phase Phase, cont Continuation, engineStep int, pending []ToolCallState, artifactIDs []string) (*AgentCheckpoint, error) {
	if r == nil || r.Checkpoints == nil {
		return nil, fmt.Errorf("agentruntime: checkpointer missing")
	}
	id := r.IdentityCopy()
	r.mu.RLock()
	model := r.Model
	r.mu.RUnlock()

	var msgs []agentctx.Message
	if r.Context != nil {
		msgs = r.Context.Replay()
	}
	cp := &AgentCheckpoint{
		TaskID:       id.TaskID,
		SessionID:    id.SessionID,
		ThreadID:     id.ThreadID,
		TurnID:       id.TurnID,
		Model:        model,
		Phase:        phase,
		Continuation: cont,
		EngineStep:   engineStep,
		Messages:     msgs,
		PendingTools: append([]ToolCallState(nil), pending...),
		ArtifactIDs:  append([]string(nil), artifactIDs...),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := r.Checkpoints.Save(ctx, cp); err != nil {
		return nil, err
	}
	return cp, nil
}
