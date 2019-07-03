package config

import "time"

// Root is starting point of the config
type Root struct {
	API API
}

// API contains configs for API endpoint
type API struct {
	ListenAddress string
	Timeout       Duration
}

// Default returns the default config
func Default() *Root {
	def := Root{
		API: API{
			ListenAddress: "/ip6/::1/tcp/1234/http",
			Timeout:       Duration(30 * time.Second),
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
