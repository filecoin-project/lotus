package main

import (
	"github.com/filecoin-project/lotus/storage/sealmgr/advmgr"
	"github.com/filecoin-project/specs-storage/storage"
)

type worker struct { // TODO: use advmgr.LocalWorker here
	*advmgr.LocalWorker
}

var _ storage.Sealer = &worker{}
