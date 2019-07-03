package config

import (
	"io"
	"os"

	"github.com/BurntSushi/toml"
)

// FromFile loads config from a specified file overriding defaults specified in
// the source code. If file does not exist or is empty defaults are asummed.
func FromFile(path string) (*Root, error) {
	file, err := os.Open(path)
	switch {
	case os.IsNotExist(err):
		return Default(), nil
	case err != nil:
		return nil, err
	}

	defer file.Close() //nolint:errcheck // The file is RO
	return FromReader(file)
}

// FromReader loads config from a reader instance.
func FromReader(reader io.Reader) (*Root, error) {
	cfg := Default()
	_, err := toml.DecodeReader(reader, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
