// Package task implements the AgentTask → Turn → Skill(Tool) layered state model.
//
//	AgentTask  顶层任务（agent_start ~ agent_end）
//	Turn       一轮推理切片（思考 + 可选多个 Skill/Tool）
//	SkillRun   Turn 内一次外部 IO（skill / tool）
//
// 三层状态独立持久化；Skill/Turn 局部失败默认不直接取消整个 AgentTask（可配置）。
package task

import (
	"encoding/json"
	"time"
)

// Status is shared across AgentTask / Turn / SkillRun.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusPaused    Status = "paused" // checkpointed; eligible for Resume
)

// Kind distinguishes skill-bound vs raw tool executions.
type Kind string

const (
	KindSkill Kind = "skill"
	KindTool  Kind = "tool"
)

// AgentTask is the root of one complete agent job.
type AgentTask struct {
	ID            string         `json:"id"`
	ThreadID      string         `json:"thread_id,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	Goal          string         `json:"goal,omitempty"`
	Status        Status         `json:"status"`
	CurrentTurnID string         `json:"current_turn_id,omitempty"`
	TurnIDs       []string       `json:"turn_ids,omitempty"`
	Error         string         `json:"error,omitempty"`
	Meta          map[string]any `json:"meta,omitempty"`
	// FailTaskOnTurnError when true cancels the task if a turn fails (default false).
	FailTaskOnTurnError bool `json:"fail_task_on_turn_error,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Turn is one reasoning slice inside an AgentTask.
type Turn struct {
	ID            string   `json:"id"`
	TaskID        string   `json:"task_id"`
	Index         int      `json:"index"`
	Status        Status   `json:"status"`
	Input         string   `json:"input,omitempty"`
	Thinking      string   `json:"thinking,omitempty"`
	AssistantText string   `json:"assistant_text,omitempty"`
	SkillRunIDs   []string `json:"skill_run_ids,omitempty"`
	Error         string   `json:"error,omitempty"`
	// FailTurnOnSkillError when true marks the turn failed if any skill fails (default false:
	// skill failure is recorded; turn may still succeed with partial results).
	FailTurnOnSkillError bool `json:"fail_turn_on_skill_error,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// SkillRun is one skill/tool IO action inside a Turn.
type SkillRun struct {
	ID      string          `json:"id"`
	TaskID  string          `json:"task_id"`
	TurnID  string          `json:"turn_id"`
	Name    string          `json:"name"` // tool name or skill name
	Kind    Kind            `json:"kind"`
	Args    json.RawMessage `json:"args,omitempty"`
	Result  string          `json:"result,omitempty"`
	Data    map[string]any  `json:"data,omitempty"`
	Status  Status          `json:"status"`
	IsError bool            `json:"is_error,omitempty"`
	Error   string          `json:"error,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Checkpoint is the resume cursor for an AgentTask.
type Checkpoint struct {
	TaskID        string    `json:"task_id"`
	CurrentTurnID string    `json:"current_turn_id,omitempty"`
	Version       int       `json:"version"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// StartRequest creates a new AgentTask.
type StartRequest struct {
	ThreadID            string
	SessionID           string
	Goal                string
	Meta                map[string]any
	FailTaskOnTurnError bool
}

// Terminal reports whether status is finished (no further work unless Resume from paused).
func (s Status) Terminal() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

func nowPtr() *time.Time {
	t := time.Now().UTC()
	return &t
}
