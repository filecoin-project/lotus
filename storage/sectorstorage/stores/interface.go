package stores

import (
	"context"
	"syscall"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type Store interface {
	AcquireSector(ctx context.Context, s abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (paths sectorbuilder.SectorPaths, stores sectorbuilder.SectorPaths, done func(), err error)
	FsStat(ctx context.Context, id ID) (FsStat, error)
}

func Stat(path string) (FsStat, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return FsStat{}, xerrors.Errorf("statfs: %w", err)
	}

	return FsStat{
		Capacity:  stat.Blocks * uint64(stat.Bsize),
		Available: stat.Bavail * uint64(stat.Bsize),
	}, nil
}

type FsStat struct {
	Capacity  uint64
	Available uint64 // Available to use for sector storage
	Used      uint64
}
