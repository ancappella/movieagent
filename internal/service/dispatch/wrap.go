package dispatch

import (
	"context"
	"fmt"

	"movieagent/internal/service/claw/provider"
	"movieagent/internal/service/claw/schema"
	clawtools "movieagent/internal/service/claw/tools"
)

// LimitedProvider wraps an LLM provider with Dispatcher LLM concurrency limits.
type LimitedProvider struct {
	Inner provider.Provider
	Disp  *Dispatcher
}

// Chat acquires an LLM slot then forwards to Inner.
func (p *LimitedProvider) Chat(ctx context.Context, req schema.ChatRequest) (*schema.ChatResponse, error) {
	if p == nil || p.Inner == nil {
		return nil, fmt.Errorf("dispatch: provider required")
	}
	release, err := p.Disp.AcquireLLM(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return p.Inner.Chat(ctx, req)
}

// Model forwards to Inner.
func (p *LimitedProvider) Model() string {
	if p == nil || p.Inner == nil {
		return ""
	}
	return p.Inner.Model()
}

// Configured forwards to Inner.
func (p *LimitedProvider) Configured() bool {
	if p == nil || p.Inner == nil {
		return false
	}
	return p.Inner.Configured()
}

// WrapProvider returns a concurrency-limited provider. disp nil → passthrough.
func WrapProvider(inner provider.Provider, disp *Dispatcher) provider.Provider {
	if disp == nil || inner == nil {
		return inner
	}
	return &LimitedProvider{Inner: inner, Disp: disp}
}

// SkillNames marks which registered tool names are skills (use skill limiter).
type SkillNames map[string]struct{}

// ToolGate returns a clawtools.Gate that applies skill vs tool limits by name.
func ToolGate(disp *Dispatcher, skills SkillNames) clawtools.Gate {
	if disp == nil {
		return nil
	}
	if skills == nil {
		skills = SkillNames{}
	}
	return func(ctx context.Context, name string) (func(), error) {
		if _, isSkill := skills[name]; isSkill {
			return disp.AcquireSkill(ctx, name)
		}
		return disp.AcquireTool(ctx, name)
	}
}
