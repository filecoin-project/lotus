package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorShutdown(t *testing.T) {
	signalCh := make(chan struct{})

	// Three shutdown handlers.
	var wg sync.WaitGroup
	wg.Add(3)
	h := ShutdownHandler{
		Component: "handler",
		StopFunc: func(_ context.Context) error {
			wg.Done()
			return nil
		},
	}

	finishCh := MonitorShutdown(signalCh, h, h, h)

	// Nothing here after 10ms.
	time.Sleep(10 * time.Millisecond)
	require.Len(t, finishCh, 0)

	// Now trigger the shutdown.
	close(signalCh)
	wg.Wait()
	<-finishCh
}
