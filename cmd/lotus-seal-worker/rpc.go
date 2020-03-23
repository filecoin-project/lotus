package main

import (
	"context"

	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/storage/sectorstorage"
)

type worker struct { // TODO: use advmgr.LocalWorker here
	*sectorstorage.LocalWorker
}

func (w *worker) Version(context.Context) (build.Version, error) {
	return build.APIVersion, nil
}

var _ storage.Sealer = &worker{}
