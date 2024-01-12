package config

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func StorageFromFile(path string, def *storiface.StorageConfig) (*storiface.StorageConfig, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, xerrors.Errorf("expanding storage config path: %w", err)
	}

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

func WriteStorageFile(filePath string, config storiface.StorageConfig) error {
	filePath, err := homedir.Expand(filePath)
	if err != nil {
		return xerrors.Errorf("expanding storage config path: %w", err)
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshaling storage config: %w", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return xerrors.Errorf("statting storage config (%s): %w", filePath, err)
		}
		if path.Base(filePath) == "." {
			filePath = path.Join(filePath, "storage.json")
		}
	} else {
		if info.IsDir() || path.Base(filePath) == "." {
			filePath = path.Join(filePath, "storage.json")
		}
	}

	if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil {
		return xerrors.Errorf("making storage config parent directory: %w", err)
	}
	if err := os.WriteFile(filePath, b, 0644); err != nil {
		return xerrors.Errorf("persisting storage config (%s): %w", filePath, err)
	}

	return nil
}
