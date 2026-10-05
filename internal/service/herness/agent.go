package herness

import (
	"context"
	"fmt"
)

// Agent is a compiled agent ready to run turns.
type Agent struct {
	Name         string
	SystemPrompt string
	Model        Model
	Tools        *Registry
	Middleware   Chain
	Checkpointer Checkpointer
	MaxSteps     int
	Features     Features
}

// Options configures NewAgent (DeerFlow create_deerflow_agent).
type Options struct {
	Name         string
	Model        Model
	Tools        []Tool
	SystemPrompt string
	Middleware   []Middleware
	Features     *Features
	Checkpointer Checkpointer
	MaxSteps     int
	// SandboxAcquire is used when Features.Sandbox is true.
	SandboxAcquire func(ctx context.Context, run *RunConfig) (sandboxID string, err error)
}

// NewAgent assembles an agent from plain arguments — no YAML required.
func NewAgent(opts Options) (*Agent, error) {
	if opts.Model == nil {
		return nil, fmt.Errorf("herness: Model is required")
	}
	features := DefaultFeatures()
	if opts.Features != nil {
		features = *opts.Features
	}

	tools := append([]Tool(nil), opts.Tools...)
	if features.BuiltinTools {
		tools = append(tools, BuiltinTools()...)
	}

	var mws []Middleware
	if opts.Middleware != nil {
		mws = append(mws, opts.Middleware...)
	} else {
		mws = DefaultMiddlewares()
		if features.Sandbox {
			mws = append([]Middleware{SandboxMiddleware{Acquire: opts.SandboxAcquire}}, mws...)
		}
		if features.LoopDetection {
			mws = append(mws, &LoopDetectionMiddleware{MaxSameTool: 4})
		}
	}

	cp := opts.Checkpointer
	if cp == nil {
		cp = NewMemoryCheckpointer()
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	name := opts.Name
	if name == "" {
		name = "default"
	}
	system := opts.SystemPrompt
	if system == "" {
		system = "You are a helpful agent. Use tools when they help complete the user request."
	}

	return &Agent{
		Name:         name,
		SystemPrompt: system,
		Model:        opts.Model,
		Tools:        NewRegistry(tools...),
		Middleware:   Chain(mws),
		Checkpointer: cp,
		MaxSteps:     maxSteps,
		Features:     features,
	}, nil
}

// Assembly pairs a compiled agent with optional observer metadata.
type Assembly struct {
	Agent      *Agent
	Descriptor map[string]any
}
