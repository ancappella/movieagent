package dispatch

import (
	"context"
	"fmt"
	"sync"
)

// Limiter caps concurrent holders with a buffered channel semaphore.
type Limiter struct {
	slots chan struct{}
	max   int
}

// NewLimiter creates a concurrency limiter. max < 1 uses DefaultConcurrency.
func NewLimiter(max int) *Limiter {
	if max < 1 {
		max = DefaultConcurrency
	}
	return &Limiter{
		slots: make(chan struct{}, max),
		max:   max,
	}
}

// Max returns the configured concurrency.
func (l *Limiter) Max() int {
	if l == nil {
		return 0
	}
	return l.max
}

// InUse returns how many slots are currently held.
func (l *Limiter) InUse() int {
	if l == nil {
		return 0
	}
	return len(l.slots)
}

// Acquire blocks until a slot is available or ctx is done.
func (l *Limiter) Acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	select {
	case l.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees one slot. Safe to call once per successful Acquire.
func (l *Limiter) Release() {
	if l == nil {
		return
	}
	select {
	case <-l.slots:
	default:
	}
}

// KeyedLimiter limits concurrency per key (e.g. each tool/skill name).
type KeyedLimiter struct {
	mu      sync.Mutex
	perKey  int
	limiters map[string]*Limiter
}

// NewKeyedLimiter creates a per-key limiter. perKey < 1 uses DefaultConcurrency.
func NewKeyedLimiter(perKey int) *KeyedLimiter {
	if perKey < 1 {
		perKey = DefaultConcurrency
	}
	return &KeyedLimiter{
		perKey:   perKey,
		limiters: make(map[string]*Limiter),
	}
}

// PerKey returns the concurrency cap for each key.
func (k *KeyedLimiter) PerKey() int {
	if k == nil {
		return 0
	}
	return k.perKey
}

func (k *KeyedLimiter) limiter(key string) *Limiter {
	if k == nil {
		return nil
	}
	if key == "" {
		key = "_"
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	lim, ok := k.limiters[key]
	if !ok {
		lim = NewLimiter(k.perKey)
		k.limiters[key] = lim
	}
	return lim
}

// Acquire holds one slot for key until Release(key) or ctx cancel.
func (k *KeyedLimiter) Acquire(ctx context.Context, key string) error {
	if k == nil {
		return nil
	}
	return k.limiter(key).Acquire(ctx)
}

// Release frees one slot for key.
func (k *KeyedLimiter) Release(key string) {
	if k == nil {
		return
	}
	k.limiter(key).Release()
}

// InUse returns current holders for key.
func (k *KeyedLimiter) InUse(key string) int {
	if k == nil {
		return 0
	}
	k.mu.Lock()
	lim := k.limiters[key]
	k.mu.Unlock()
	if lim == nil {
		return 0
	}
	return lim.InUse()
}

// ErrLimited is returned when a non-blocking try fails (reserved for future APIs).
var ErrLimited = fmt.Errorf("dispatch: concurrency limit reached")
