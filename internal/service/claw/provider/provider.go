package provider

import (
	"context"

	"movieagent/internal/service/claw/schema"
)

// Provider is the LLM adapter used by the claw engine.
// req.Tools == nil means Thinking (no tool schemas).
type Provider interface {
	Chat(ctx context.Context, req schema.ChatRequest) (*schema.ChatResponse, error)
	Model() string
	Configured() bool
}
