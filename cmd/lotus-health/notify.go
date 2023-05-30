package main

import (
	"context"
	"os"

	"github.com/coreos/go-systemd/v22/dbus"
)

func notifyHandler(ctx context.Context, n string, ch chan interface{}, sCh chan os.Signal) (string, error) {
	select {
	// alerts to restart systemd unit
	case <-ch:
		statusCh := make(chan string, 1)
		c, err := dbus.NewWithContext(ctx)
		if err != nil {
			return "", err
		}
		_, err = c.TryRestartUnitContext(ctx, n, "fail", statusCh)
		if err != nil {
			return "", err
		}
		select {
		case result := <-statusCh:
			return result, nil
		}
	// SIGTERM
	case <-sCh:
		os.Exit(1)
		return "", nil
	}
}
