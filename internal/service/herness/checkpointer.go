package herness

import (
	"context"
	"sync"
	"time"
)

// Checkpointer persists ThreadState between turns.
type Checkpointer interface {
	Get(ctx context.Context, threadID string) (ThreadState, bool, error)
	Put(ctx context.Context, threadID string, state ThreadState) error
	Delete(ctx context.Context, threadID string) error
}

// MemoryCheckpointer is an in-process checkpointer (DeerFlow memory saver).
type MemoryCheckpointer struct {
	mu    sync.RWMutex
	store map[string]ThreadState
}

func NewMemoryCheckpointer() *MemoryCheckpointer {
	return &MemoryCheckpointer{store: map[string]ThreadState{}}
}

func (c *MemoryCheckpointer) Get(_ context.Context, threadID string) (ThreadState, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, ok := c.store[threadID]
	if !ok {
		return ThreadState{}, false, nil
	}
	return state.Clone(), true, nil
}

func (c *MemoryCheckpointer) Put(_ context.Context, threadID string, state ThreadState) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state.UpdatedAt = time.Now().UTC()
	c.store[threadID] = state.Clone()
	return nil
}

func (c *MemoryCheckpointer) Delete(_ context.Context, threadID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, threadID)
	return nil
}
