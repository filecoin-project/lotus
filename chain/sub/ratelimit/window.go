package ratelimit

import "time"

// Window is a time windows for counting events within a span of time.  The
// windows slides forward in time so that it spans from the most recent event
// to size time in the past.
type Window struct {
	q    *queue
	size int64
}

// NewWindow creates a new Window that limits the number of events to maximum
// count of events within a duration of time.  The capacity sets the maximum
// number of events, and size sets the span of time over which the events are
// counted.
func NewWindow(capacity int, size time.Duration) *Window {
	return &Window{
		q: &queue{
			buf: make([]int64, capacity),
		},
		size: int64(size),
	}
}

// Add attempts to append a new timestamp into the current window.  Previously
// added values that are not within `size` difference from the value being
// added are first removed.  Add fails if adding the value would cause the
// window to exceed capacity.
func (w *Window) Add() error {
	now := time.Now().UnixNano()
	if w.Len() != 0 {
		w.q.truncate(now - w.size)
	}
	return w.q.push(now)
}

// Cap returns the maximum number of items the window can hold.
func (w *Window) Cap() int {
	return w.q.cap()
}

// Len returns the number of elements currently in the window.
func (w *Window) Len() int {
	return w.q.len()
}

// Span returns the distance from the first to the last item in the window.
func (w *Window) Span() time.Duration {
	if w.q.len() < 2 {
		return 0
	}
	return time.Duration(w.q.back() - w.q.front())
}

// Oldest returns the oldest timestamp in the window.
func (w *Window) Oldest() time.Time {
	if w.q.len() == 0 {
		return time.Time{}
	}
	return time.Unix(0, w.q.front())
}

// Newest returns the newest timestamp in the window.
func (w *Window) Newest() time.Time {
	if w.q.len() == 0 {
		return time.Time{}
	}
	return time.Unix(0, w.q.back())
}
