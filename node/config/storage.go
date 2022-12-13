package config

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func StorageFromFile(path string, def *storiface.StorageConfig) (*storiface.StorageConfig, error) {
	file, err := os.Open(path)
	switch {
	case os.IsNotExist(err):
		if def == nil {
			return nil, xerrors.Errorf("couldn't load storage config: %w", err)
		}
		return def, nil
	case err != nil:
		return nil, err
	}

	defer file.Close() //nolint:errcheck // The file is RO
	return StorageFromReader(file)
}

func StorageFromReader(reader io.Reader) (*storiface.StorageConfig, error) {
	var cfg storiface.StorageConfig
	err := json.NewDecoder(reader).Decode(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func WriteStorageFile(path string, config storiface.StorageConfig) error {
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshaling storage config: %w", err)
	}

	if err := ioutil.WriteFile(path, b, 0644); err != nil {
		return xerrors.Errorf("persisting storage config (%s): %w", path, err)
	}

	return nil
}
