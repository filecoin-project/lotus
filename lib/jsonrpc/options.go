package jsonrpc

import (
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	ReconnectInterval time.Duration

	proxyConnFactory func(func() (*websocket.Conn, error)) func() (*websocket.Conn, error) // for testing
}

var defaultConfig = Config{
	ReconnectInterval: time.Second * 5,
}

type Option func(c *Config)

func WithReconnectInterval(d time.Duration) func(c *Config) {
	return func(c *Config) {
		c.ReconnectInterval = d
	}
}
