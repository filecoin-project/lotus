package main

import (
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/go-sectorbuilder"
)

type worker struct { // TODO: use advmgr.LocalWorker here
	sectorbuilder.Basic
}

var _ storage.Sealer = &worker{}
