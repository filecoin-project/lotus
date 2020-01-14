package main

import (
	"github.com/coreos/go-systemd/dbus"
)

func alertHandler(n string, ch chan interface{}) (string, error) {
	select {
	case <-ch:
		statusCh := make(chan string, 1)
		c, err := dbus.New()
		if err != nil {
			return "", err
		}
		_, err = c.TryRestartUnit(n, "fail", statusCh)
		if err != nil {
			return "", err
		}
		select {
		case result := <-statusCh:
			return result, nil
		}
	}
}
