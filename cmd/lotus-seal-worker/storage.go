package main

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
)

type workerStorage struct {
	path string // TODO: multi-path support

	api api.StorageMiner
}

func (w *workerStorage) AcquireSector(ctx context.Context, id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	w.api.WorkerFindSector()
}

var _ sectorbuilder.SectorProvider = &workerStorage{}
