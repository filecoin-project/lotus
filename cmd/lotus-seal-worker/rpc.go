package main

import (
	"context"

	"github.com/filecoin-project/specs-storage/storage"

	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"

	"github.com/filecoin-project/lotus/build"
)

type worker struct {
	*sectorstorage.LocalWorker
}

func (w *worker) Version(context.Context) (build.Version, error) {
	return build.APIVersion, nil
}

var _ storage.Sealer = &worker{}
