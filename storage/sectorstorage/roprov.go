package sectorstorage

import (
	"context"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"
)

type readonlyProvider struct {
	stor *stores.Local
}

func (l *readonlyProvider) AcquireSector(ctx context.Context, id abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	if allocate != stores.FTNone {
		return sectorbuilder.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	p, _, done, err := l.stor.AcquireSector(ctx, id, existing, allocate, sealing)

	return p, done, err
}
