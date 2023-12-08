package promise

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPromiseSet(t *testing.T) {
	p := &Promise[int]{}

	p.Set(42)
	if p.val != 42 {
		t.Fatalf("expected 42, got %v", p.val)
	}
}

func TestPromiseVal(t *testing.T) {
	p := &Promise[int]{}

	p.Set(42)

	ctx := context.Background()
	val := p.Val(ctx)

	if val != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestPromiseValWaitsForSet(t *testing.T) {
	p := &Promise[int]{}
	var val int

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		ctx := context.Background()
		val = p.Val(ctx)
	}()

	time.Sleep(100 * time.Millisecond) // Give some time for the above goroutine to execute
	p.Set(42)
	wg.Wait()

	if val != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestPromiseValContextCancel(t *testing.T) {
	p := &Promise[int]{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context

	val := p.Val(ctx)

	var zeroValue int
	if val != zeroValue {
		t.Fatalf("expected zero-value, got %v", val)
	}
}
