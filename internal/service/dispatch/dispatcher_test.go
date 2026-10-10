package dispatch

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatcherCapsConcurrentTasks(t *testing.T) {
	d := New(Config{MaxTasks: 2, MaxLLM: 3, MaxTool: 3, MaxSkill: 3})
	var running atomic.Int32
	var peak atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := d.Run(context.Background(), func(ctx context.Context) error {
				n := running.Add(1)
				for {
					old := peak.Load()
					if n <= old || peak.CompareAndSwap(old, n) {
						break
					}
				}
				time.Sleep(30 * time.Millisecond)
				running.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("Run: %v", err)
			}
		}()
	}
	wg.Wait()
	if peak.Load() > 2 {
		t.Fatalf("peak concurrent=%d want ≤2", peak.Load())
	}
}

func TestKeyedLimiterPerName(t *testing.T) {
	k := NewKeyedLimiter(1)
	ctx := context.Background()
	if err := k.Acquire(ctx, "echo"); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() {
		c, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		defer cancel()
		blocked <- k.Acquire(c, "echo")
	}()
	if err := <-blocked; err == nil {
		t.Fatal("expected block on same key")
	}
	// Different key should not block.
	if err := k.Acquire(ctx, "add"); err != nil {
		t.Fatal(err)
	}
	k.Release("add")
	k.Release("echo")
}

func TestAcquireLLMRelease(t *testing.T) {
	d := New(Config{MaxTasks: 3, MaxLLM: 1, MaxTool: 3, MaxSkill: 3})
	rel, err := d.AcquireLLM(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := d.AcquireLLM(ctx); err == nil {
		t.Fatal("expected LLM limit")
	}
	rel()
	rel2, err := d.AcquireLLM(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rel2()
}
