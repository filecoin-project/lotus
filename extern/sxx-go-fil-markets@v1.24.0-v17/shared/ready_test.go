package shared

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestReadyManager(t *testing.T) {
	rm := NewReadyManager()

	// Verify that AwaitReady blocks until FireReady is called
	awaitReadyComplete := make(chan error)
	go func() {
		err := rm.AwaitReady()
		awaitReadyComplete <- err
	}()
	select {
	case <-awaitReadyComplete:
		require.Fail(t, "AwaitReady should block until FireReady is called")
	default:
	}

	// Verify that OnReady is called with the error supplied to FireReady
	errs := make(chan error)
	rm.OnReady(func(err error) {
		errs <- err
	})

	someErr := xerrors.Errorf("some err")
	go rm.FireReady(someErr)

	select {
	case err := <-errs:
		require.Equal(t, someErr, err)
	}

	select {
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "AwaitReady should complete when FireReady is called")
	case err := <-awaitReadyComplete:
		require.Equal(t, someErr, err)
	}

	// Verify that calling OnReady after FireReady has been called results
	// in the callback being called immediately
	errs2 := make(chan error)
	rm.OnReady(func(err error) {
		errs2 <- err
	})

	select {
	case err := <-errs2:
		require.Equal(t, someErr, err)
	}

	// Verify that calling AwaitReady after FireReady has been called returns
	// immediately
	rm.AwaitReady()
	t.Log("AwaitReady complete")
}
