package herness

import (
	"context"
	"fmt"
)

// Client is the embedded SDK entry point (DeerFlow DeerFlowClient).
type Client struct {
	Agent  *Agent
	Runner *Runner
}

// NewClient wraps a compiled agent.
func NewClient(agent *Agent) *Client {
	return &Client{Agent: agent, Runner: NewRunner(agent)}
}

// Chat runs one turn and returns the final assistant text.
func (c *Client) Chat(ctx context.Context, message, threadID string) (string, error) {
	res, err := c.Runner.RunTurn(ctx, RunConfig{ThreadID: threadID}, message, nil)
	if err != nil {
		return "", err
	}
	return res.Response, nil
}

// Stream runs one turn and returns a channel of StreamEvents.
// The channel is closed when the turn finishes.
func (c *Client) Stream(ctx context.Context, message, threadID string) (<-chan StreamEvent, error) {
	if c == nil || c.Runner == nil {
		return nil, fmt.Errorf("herness: client has no agent")
	}
	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		_, err := c.Runner.RunTurn(ctx, RunConfig{ThreadID: threadID}, message, func(ev StreamEvent) {
			select {
			case <-ctx.Done():
			case ch <- ev:
			}
		})
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- StreamEvent{Type: EventError, Data: map[string]any{"error": err.Error()}}:
			}
		}
	}()
	return ch, nil
}

// Reset clears checkpoint state for a thread.
func (c *Client) Reset(ctx context.Context, threadID string) error {
	if c == nil || c.Agent == nil || c.Agent.Checkpointer == nil {
		return nil
	}
	return c.Agent.Checkpointer.Delete(ctx, threadID)
}
