package herness

import "context"

// ModelRequest is what middleware and the loop pass to the model.
type ModelRequest struct {
	Messages     []Message
	Tools        []ToolSchema
	SystemPrompt string
}

// ModelResponse is one model step result.
type ModelResponse struct {
	Message Message
}

// Model is the chat model interface used by the agent loop.
type Model interface {
	Chat(ctx context.Context, req ModelRequest) (ModelResponse, error)
}

// ModelFunc adapts a function into a Model.
type ModelFunc func(ctx context.Context, req ModelRequest) (ModelResponse, error)

func (f ModelFunc) Chat(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	return f(ctx, req)
}
