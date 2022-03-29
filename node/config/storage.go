package config

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/xerrors"

	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
)

func StorageFromFile(path string, def *stores.StorageConfig) (*stores.StorageConfig, error) {
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

func StorageFromReader(reader io.Reader) (*stores.StorageConfig, error) {
	var cfg stores.StorageConfig
	err := json.NewDecoder(reader).Decode(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func WriteStorageFile(path string, config stores.StorageConfig) error {
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshaling storage config: %w", err)
	}

	if err := ioutil.WriteFile(path, b, 0644); err != nil {
		return xerrors.Errorf("persisting storage config (%s): %w", path, err)
	}

	return nil
}

func (c *StorageMiner) StorageManager() sectorstorage.Config {
	return sectorstorage.Config{
		ParallelFetchLimit:       c.Storage.ParallelFetchLimit,
		AllowAddPiece:            c.Storage.AllowAddPiece,
		AllowPreCommit1:          c.Storage.AllowPreCommit1,
		AllowPreCommit2:          c.Storage.AllowPreCommit2,
		AllowCommit:              c.Storage.AllowCommit,
		AllowUnseal:              c.Storage.AllowUnseal,
		AllowReplicaUpdate:       c.Storage.AllowReplicaUpdate,
		AllowProveReplicaUpdate2: c.Storage.AllowProveReplicaUpdate2,
		AllowRegenSectorKey:      c.Storage.AllowRegenSectorKey,
		ResourceFiltering:        c.Storage.ResourceFiltering,

		ParallelCheckLimit: c.Proving.ParallelCheckLimit,
	}
}
