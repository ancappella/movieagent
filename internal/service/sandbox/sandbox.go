package sandbox

import "context"

const (
	UserDataVirtualPrefix      = "/mnt/user-data"
	ACPWorkspaceVirtualPrefix  = "/mnt/acp-workspace"
	DefaultSkillsContainerPath = "/mnt/skills"
	DefaultCommandTimeoutSec   = 600
)

// GrepMatch is one line hit from Sandbox.Grep.
type GrepMatch struct {
	Path       string
	LineNumber int
	Line       string
}

// Sandbox is the execution and filesystem contract seen by agents.
// Virtual paths such as /mnt/user-data/... are translated by the implementation.
type Sandbox interface {
	ID() string
	ExecuteCommand(ctx context.Context, command string, env map[string]string) (string, error)
	ReadFile(path string, startLine, endLine int) (string, error)
	WriteFile(path, content string, append bool) error
	ListDir(path string, maxDepth int) ([]string, error)
}
