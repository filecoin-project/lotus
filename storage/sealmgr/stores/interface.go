package stores

import (
	"context"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type Store interface {
	AcquireSector(ctx context.Context, s abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (paths sectorbuilder.SectorPaths, stores sectorbuilder.SectorPaths, done func(), err error)
}
