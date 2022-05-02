package ratelimit

import (
	"testing"
	"time"
)

func TestWindow(t *testing.T) {
	const (
		maxEvents = 3
		timeLimit = 100 * time.Millisecond
	)
	w := NewWindow(maxEvents, timeLimit)
	if w.Len() != 0 {
		t.Fatal("q.Len() =", w.Len(), "expect 0")
	}
	if w.Cap() != maxEvents {
		t.Fatal("q.Cap() =", w.Cap(), "expect 3")
	}
	if !w.Newest().IsZero() {
		t.Fatal("expected newest to be zero time with empty window")
	}
	if !w.Oldest().IsZero() {
		t.Fatal("expected oldest to be zero time with empty window")
	}
	if w.Span() != 0 {
		t.Fatal("expected span to be zero time with empty window")
	}

	var err error
	for i := 0; i < maxEvents; i++ {
		err = w.Add()
		if err != nil {
			t.Fatalf("cannot add event %d", i)
		}
	}
	if w.Len() != maxEvents {
		t.Fatalf("q.Len() is %d, expected %d", w.Len(), maxEvents)
	}
	if err = w.Add(); err != ErrRateLimitExceeded {
		t.Fatalf("add event %d within time limit should have failed with err: %s", maxEvents+1, ErrRateLimitExceeded)
	}

	time.Sleep(timeLimit)
	if err = w.Add(); err != nil {
		t.Fatalf("cannot add event after time limit: %s", err)
	}

	prev := w.Newest()
	time.Sleep(timeLimit)
	err = w.Add()
	if err != nil {
		t.Fatalf("cannot add event")
	}
	if w.Newest().Before(prev) {
		t.Fatal("newest is before previous value")
	}
	if w.Oldest().Before(prev) {
		t.Fatal("oldest is before previous value")
	}
}
