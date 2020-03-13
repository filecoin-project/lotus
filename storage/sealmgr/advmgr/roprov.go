package advmgr

import (
	"context"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"
)

type readonlyProvider struct {
	miner abi.ActorID
	stor  *stores.Local
}

func (l *readonlyProvider) AcquireSector(ctx context.Context, id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	if allocate != 0 {
		return sectorbuilder.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	return l.stor.AcquireSector(ctx, abi.SectorID{
		Miner: l.miner,
		Number: id,
	}, existing, allocate, sealing)
}
