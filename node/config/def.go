package config

import "time"

// Root is starting point of the config
type Root struct {
	API    API
	Libp2p Libp2p

	Metrics Metrics
}

// API contains configs for API endpoint
type API struct {
	ListenAddress string
	Timeout       Duration
}

// Libp2p contains configs for libp2p
type Libp2p struct {
	ListenAddresses []string
	BootstrapPeers  []string
}

type Metrics struct {
	Nickname string
}

// Default returns the default config
func Default() *Root {
	def := Root{
		API: API{
			ListenAddress: "/ip6/::1/tcp/1234/http",
			Timeout:       Duration(30 * time.Second),
		},
		Libp2p: Libp2p{
			ListenAddresses: []string{
				"/ip4/0.0.0.0/tcp/0",
				"/ip6/::/tcp/0",
			},
		},
	}
	return &def
}

// Duration is a wrapper type for time.Duration for decoding it from TOML
type Duration time.Duration

// UnmarshalText implements interface for TOML decoding
func (dur *Duration) UnmarshalText(text []byte) error {
	d, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*dur = Duration(d)
	return err
}
