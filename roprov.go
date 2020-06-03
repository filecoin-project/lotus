package sectorstorage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/stores"
)

type readonlyProvider struct {
	index stores.SectorIndex
	stor  *stores.Local
	spt   abi.RegisteredProof
}

func (l *readonlyProvider) AcquireSector(ctx context.Context, id abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, sealing bool) (stores.SectorPaths, func(), error) {
	if allocate != stores.FTNone {
		return stores.SectorPaths{}, nil, xerrors.New("read-only storage")
	}

	ctx, cancel := context.WithCancel(ctx)
	if err := l.index.StorageLock(ctx, id, existing, stores.FTNone); err != nil {
		return stores.SectorPaths{}, nil, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	p, _, done, err := l.stor.AcquireSector(ctx, id, l.spt, existing, allocate, stores.PathType(sealing), stores.AcquireMove)

	return p, func() {
		cancel()
		done()
	}, err
}
