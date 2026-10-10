package dispatch

import (
	"context"
	"sync"
)

// Job is work scheduled by the Dispatcher after a global task slot is acquired.
type Job func(ctx context.Context) error

// Dispatcher schedules AgentTasks within MaxTasks and exposes LLM/tool/skill limiters.
type Dispatcher struct {
	cfg   Config
	tasks *Limiter
	LLM   *Limiter
	Tool  *KeyedLimiter
	Skill *KeyedLimiter

	mu      sync.Mutex
	running int
	queued  int
}

// New builds a Dispatcher from cfg (zeros → DefaultConcurrency).
func New(cfg Config) *Dispatcher {
	cfg = cfg.normalized()
	return &Dispatcher{
		cfg:   cfg,
		tasks: NewLimiter(cfg.MaxTasks),
		LLM:   NewLimiter(cfg.MaxLLM),
		Tool:  NewKeyedLimiter(cfg.MaxTool),
		Skill: NewKeyedLimiter(cfg.MaxSkill),
	}
}

// Default returns a Dispatcher with all limits at DefaultConcurrency.
func Default() *Dispatcher {
	return New(DefaultConfig())
}

// Config returns a copy of the active limits.
func (d *Dispatcher) Config() Config {
	if d == nil {
		return DefaultConfig()
	}
	return d.cfg
}

// Stats snapshots queue pressure for diagnostics.
func (d *Dispatcher) Stats() (running, queued, maxTasks int) {
	if d == nil {
		return 0, 0, 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running, d.queued, d.cfg.MaxTasks
}

// Run acquires a global task slot, runs job, then releases.
// Callers that already recorded a pending AgentTask should Activate it inside job.
func (d *Dispatcher) Run(ctx context.Context, job Job) error {
	if d == nil {
		if job == nil {
			return nil
		}
		return job(ctx)
	}
	if job == nil {
		return nil
	}

	d.mu.Lock()
	d.queued++
	d.mu.Unlock()

	err := d.tasks.Acquire(ctx)

	d.mu.Lock()
	d.queued--
	if err == nil {
		d.running++
	}
	d.mu.Unlock()

	if err != nil {
		return err
	}
	defer func() {
		d.tasks.Release()
		d.mu.Lock()
		d.running--
		d.mu.Unlock()
	}()

	return job(ctx)
}

// AcquireLLM holds one LLM slot until the returned release func is called.
func (d *Dispatcher) AcquireLLM(ctx context.Context) (release func(), err error) {
	if d == nil || d.LLM == nil {
		return func() {}, nil
	}
	if err := d.LLM.Acquire(ctx); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(d.LLM.Release) }, nil
}

// AcquireTool holds one slot for the named tool.
func (d *Dispatcher) AcquireTool(ctx context.Context, name string) (release func(), err error) {
	if d == nil || d.Tool == nil {
		return func() {}, nil
	}
	if err := d.Tool.Acquire(ctx, name); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { d.Tool.Release(name) }) }, nil
}

// AcquireSkill holds one slot for the named skill.
func (d *Dispatcher) AcquireSkill(ctx context.Context, name string) (release func(), err error) {
	if d == nil || d.Skill == nil {
		return func() {}, nil
	}
	if err := d.Skill.Acquire(ctx, name); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { d.Skill.Release(name) }) }, nil
}
