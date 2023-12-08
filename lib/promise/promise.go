package promise

import (
	"context"
	"sync"
)

type Promise[T any] struct {
	val  T
	done chan struct{}
	mu   sync.Mutex
}

func (p *Promise[T]) Set(val T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Set value
	p.val = val

	// Initialize the done channel if it hasn't been initialized
	if p.done == nil {
		p.done = make(chan struct{})
	}

	// Signal that the value is set
	close(p.done)
}

func (p *Promise[T]) Val(ctx context.Context) T {
	p.mu.Lock()
	// Initialize the done channel if it hasn't been initialized
	if p.done == nil {
		p.done = make(chan struct{})
	}
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		return *new(T)
	case <-p.done:
		p.mu.Lock()
		val := p.val
		p.mu.Unlock()
		return val
	}
}
