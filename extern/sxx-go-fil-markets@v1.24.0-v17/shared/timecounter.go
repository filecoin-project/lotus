package shared

import (
	"sync/atomic"
	"time"
)

// timeCounter is used to generate a monotonically increasing sequence.
// It starts at the current time, then increments on each call to next.
type TimeCounter struct {
	counter uint64
}

func NewTimeCounter() *TimeCounter {
	return &TimeCounter{counter: uint64(time.Now().UnixNano())}
}

func (tc *TimeCounter) Next() uint64 {
	counter := atomic.AddUint64(&tc.counter, 1)
	return counter
}
