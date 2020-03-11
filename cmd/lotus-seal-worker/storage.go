package main

import (
	"context"
	"net/http"
	"sort"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

type workerStorage struct {
	path string // TODO: multi-path support
	mid abi.ActorID

	auth http.Header
	api api.StorageMiner
}

func (w *workerStorage) AcquireSector(ctx context.Context, id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	asid := abi.SectorID{
		Miner:  w.mid,
		Number: id,
	}

	// extract local storage; prefer

	si, err := w.api.WorkerFindSector(ctx, asid, existing)
	if err != nil {
		return sectorbuilder.SectorPaths{}, nil, err
	}

	sort.Slice(si, func(i, j int) bool {
		return si[i].Cost < si[j].Cost
	})

	best := si[0].URLs // TODO: not necessarily true

	sname := sectorutil.SectorName(abi.SectorID{
		Miner:  w.mid,
		Number: id,
	})

	w.fetch(best, )
}

var _ sectorbuilder.SectorProvider = &workerStorage{}
