package herness

import "context"

// Middleware hooks into the agent loop (DeerFlow AgentMiddleware subset).
//
// Ordering: first in the slice is outermost. WrapModelCall / WrapToolCall
// nest as onion layers around the core call.
type Middleware interface {
	Name() string
	BeforeAgent(ctx context.Context, state *ThreadState, run *RunConfig) error
	AfterAgent(ctx context.Context, state *ThreadState, run *RunConfig) error
	WrapModelCall(ctx context.Context, req *ModelRequest, next ModelHandler) (ModelResponse, error)
	WrapToolCall(ctx context.Context, call ToolCall, next ToolHandler) (ToolResult, error)
}

// ModelHandler is the next model step in a middleware chain.
type ModelHandler func(ctx context.Context, req *ModelRequest) (ModelResponse, error)

// ToolHandler is the next tool step in a middleware chain.
type ToolHandler func(ctx context.Context, call ToolCall) (ToolResult, error)

// BaseMiddleware provides no-op defaults so partial middlewares can embed it.
type BaseMiddleware struct{}

func (BaseMiddleware) BeforeAgent(context.Context, *ThreadState, *RunConfig) error { return nil }
func (BaseMiddleware) AfterAgent(context.Context, *ThreadState, *RunConfig) error  { return nil }
func (BaseMiddleware) WrapModelCall(ctx context.Context, req *ModelRequest, next ModelHandler) (ModelResponse, error) {
	return next(ctx, req)
}
func (BaseMiddleware) WrapToolCall(ctx context.Context, call ToolCall, next ToolHandler) (ToolResult, error) {
	return next(ctx, call)
}

// Chain composes middlewares. Index 0 is outermost.
type Chain []Middleware

func (c Chain) beforeAgent(ctx context.Context, state *ThreadState, run *RunConfig) error {
	for _, mw := range c {
		if mw == nil {
			continue
		}
		if err := mw.BeforeAgent(ctx, state, run); err != nil {
			return err
		}
	}
	return nil
}

func (c Chain) afterAgent(ctx context.Context, state *ThreadState, run *RunConfig) error {
	for i := len(c) - 1; i >= 0; i-- {
		if c[i] == nil {
			continue
		}
		if err := c[i].AfterAgent(ctx, state, run); err != nil {
			return err
		}
	}
	return nil
}

func (c Chain) model(core ModelHandler) ModelHandler {
	h := core
	for i := len(c) - 1; i >= 0; i-- {
		mw := c[i]
		if mw == nil {
			continue
		}
		next := h
		h = func(ctx context.Context, req *ModelRequest) (ModelResponse, error) {
			return mw.WrapModelCall(ctx, req, next)
		}
	}
	return h
}

func (c Chain) tool(core ToolHandler) ToolHandler {
	h := core
	for i := len(c) - 1; i >= 0; i-- {
		mw := c[i]
		if mw == nil {
			continue
		}
		next := h
		h = func(ctx context.Context, call ToolCall) (ToolResult, error) {
			return mw.WrapToolCall(ctx, call, next)
		}
	}
	return h
}
