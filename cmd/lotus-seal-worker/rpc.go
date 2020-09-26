package main

import (
	"context"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/build"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
)

type worker struct {
	*sectorstorage.LocalWorker

	localStore *stores.Local
	ls         stores.LocalStorage
}

func (w *worker) Version(context.Context) (build.Version, error) {
	return build.WorkerAPIVersion, nil
}

func (w *worker) StorageAddLocal(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	if err := w.localStore.OpenPath(ctx, path); err != nil {
		return xerrors.Errorf("opening local path: %w", err)
	}

	if err := w.ls.SetStorage(func(sc *stores.StorageConfig) {
		sc.StoragePaths = append(sc.StoragePaths, stores.LocalPath{Path: path})
	}); err != nil {
		return xerrors.Errorf("get storage config: %w", err)
	}

	return nil
}

var _ storage.Sealer = &worker{}
