package task

import "context"

// Store persists AgentTask / Turn / SkillRun independently.
type Store interface {
	SaveTask(ctx context.Context, t *AgentTask) error
	GetTask(ctx context.Context, id string) (*AgentTask, error)
	ListTasks(ctx context.Context, threadID string) ([]*AgentTask, error)

	SaveTurn(ctx context.Context, t *Turn) error
	GetTurn(ctx context.Context, id string) (*Turn, error)
	ListTurns(ctx context.Context, taskID string) ([]*Turn, error)

	SaveSkill(ctx context.Context, s *SkillRun) error
	GetSkill(ctx context.Context, id string) (*SkillRun, error)
	ListSkills(ctx context.Context, turnID string) ([]*SkillRun, error)

	SaveCheckpoint(ctx context.Context, cp *Checkpoint) error
	GetCheckpoint(ctx context.Context, taskID string) (*Checkpoint, error)
}
