package stores

import (
	"context"
	"syscall"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type PathType string

const (
	PathStorage = "storage"
	PathSealing = "sealing"
)

type AcquireMode string

const (
	AcquireMove = "move"
	AcquireCopy = "copy"
)

type Store interface {
	AcquireSector(ctx context.Context, s abi.SectorID, spt abi.RegisteredSealProof, existing SectorFileType, allocate SectorFileType, sealing PathType, op AcquireMode) (paths SectorPaths, stores SectorPaths, err error)
	Remove(ctx context.Context, s abi.SectorID, types SectorFileType, force bool) error

	// like remove, but doesn't remove the primary sector copy, nor the last
	// non-primary copy if there no primary copies
	RemoveCopies(ctx context.Context, s abi.SectorID, types SectorFileType) error

	// move sectors into storage
	MoveStorage(ctx context.Context, s abi.SectorID, spt abi.RegisteredSealProof, types SectorFileType) error

	FsStat(ctx context.Context, id ID) (FsStat, error)
}

func Stat(path string) (FsStat, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return FsStat{}, xerrors.Errorf("statfs: %w", err)
	}

	return FsStat{
		Capacity:  int64(stat.Blocks) * stat.Bsize,
		Available: int64(stat.Bavail) * stat.Bsize,
	}, nil
}

type FsStat struct {
	Capacity  int64
	Available int64 // Available to use for sector storage
	Used      int64
	Reserved  int64
}
