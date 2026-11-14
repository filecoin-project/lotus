package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/api"
)

func TestRetryCancelsBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	calls := 0
	go func() {
		_, err := Retry(ctx, 2, time.Minute, []error{new(api.ErrOutOfGas)}, func() (int, error) {
			calls++
			return 0, new(api.ErrOutOfGas)
		})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline error, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected one attempt, got %d", calls)
		}
	case <-time.After(time.Second):
		t.Fatal("Retry did not stop when its context expired during backoff")
	}
}
