package main

import (
	"context"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/storage/sealmgr/advmgr"
	"github.com/filecoin-project/specs-storage/storage"
)

type worker struct { // TODO: use advmgr.LocalWorker here
	*advmgr.LocalWorker
}

func (w *worker) Version(context.Context) (build.Version, error) {
	return build.APIVersion, nil
}

var _ storage.Sealer = &worker{}
