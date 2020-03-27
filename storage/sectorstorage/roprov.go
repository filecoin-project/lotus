package sectorstorage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"
)

type readonlyProvider struct {
	stor *stores.Local
}

func (l *readonlyProvider) AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, sealing bool) (stores.SectorPaths, func(), error) {
	if allocate != stores.FTNone {
		return stores.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	p, _, done, err := l.stor.AcquireSector(ctx, id, existing, allocate, sealing)

	return p, done, err
}
