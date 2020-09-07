package stores

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
)

type PathType string

const (
	PathStorage PathType = "storage"
	PathSealing PathType = "sealing"
)

type AcquireMode string

const (
	AcquireMove AcquireMode = "move"
	AcquireCopy AcquireMode = "copy"
)

type Store interface {
	AcquireSector(ctx context.Context, s abi.SectorID, spt abi.RegisteredSealProof, existing SectorFileType, allocate SectorFileType, sealing PathType, op AcquireMode) (paths SectorPaths, stores SectorPaths, err error)
	Remove(ctx context.Context, s abi.SectorID, types SectorFileType, force bool) error

	// like remove, but doesn't remove the primary sector copy, nor the last
	// non-primary copy if there no primary copies
	RemoveCopies(ctx context.Context, s abi.SectorID, types SectorFileType) error

	// move sectors into storage
	MoveStorage(ctx context.Context, s abi.SectorID, spt abi.RegisteredSealProof, types SectorFileType) error

	FsStat(ctx context.Context, id ID) (fsutil.FsStat, error)
}
