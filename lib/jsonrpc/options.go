package jsonrpc

import "time"

type Config struct {
	ReconnectInterval time.Duration
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
