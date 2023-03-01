package node

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

type ShutdownHandler struct {
	Component string
	StopFunc  StopFunc
}

// MonitorShutdown manages shutdown requests, by watching signals and invoking
// the supplied handlers in order.
//
// It watches SIGTERM and SIGINT OS signals, as well as the trigger channel.
// When any of them fire, it calls the supplied handlers in order. If any of
// them errors, it merely logs the error.
//
// Once the shutdown has completed, it closes the returned channel. The caller
// can watch this channel
func MonitorShutdown(triggerCh <-chan struct{}, handlers ...ShutdownHandler) <-chan struct{} {
	sigCh := make(chan os.Signal, 2)
	out := make(chan struct{})

	go func() {
		select {
		case sig := <-sigCh:
			log.Warnw("received shutdown", "signal", sig)
		case <-triggerCh:
			log.Warn("received shutdown")
		}

		log.Warn("Shutting down...")

		// Call all the handlers, logging on failure and success.
		for _, h := range handlers {
			if err := h.StopFunc(context.TODO()); err != nil {
				log.Errorf("shutting down %s failed: %s", h.Component, err)
				continue
			}
			log.Infof("%s shut down successfully ", h.Component)
		}

		log.Warn("Graceful shutdown successful")

		// Sync all loggers.
		_ = log.Sync() //nolint:errcheck
		close(out)
	}()

	signal.Reset(syscall.SIGTERM, syscall.SIGINT)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	return out
}
