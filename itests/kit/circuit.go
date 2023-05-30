package kit

import (
	"fmt"
	"testing"
	"time"
)

/*
CircuitBreaker implements a simple time-based circuit breaker used for waiting for async operations to finish.

This is how it works:
  - It runs the `cb` function until it returns true,
  - waiting for `throttle` duration between each iteration,
  - or at most `timeout` duration until it breaks test execution.

You can use it if t.Deadline() is not "granular" enough, and you want to know which specific piece of code timed out,
or you need to set different deadlines in the same test.
*/
func CircuitBreaker(t *testing.T, label string, throttle, timeout time.Duration, cb func() bool) {
	tmo := time.After(timeout)
	for {
		if cb() {
			break
		}
		select {
		case <-tmo:
			t.Fatal("timeout: ", label)
		default:
			fmt.Printf("waiting: %s\n", label)
			time.Sleep(throttle)
		}
	}
}
