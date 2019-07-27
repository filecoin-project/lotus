package impl

import (
	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
)

type StorageMinerAPI struct {
	CommonAPI

	SectorBuilder *sectorbuilder.SectorBuilder
}

var _ api.StorageMiner = &StorageMinerAPI{}
