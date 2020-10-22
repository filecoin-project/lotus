package sectorstorage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
)

type existingSelector struct {
	index      stores.SectorIndex
	sector     abi.SectorID
	exist      stores.SectorFileType
	ptype      stores.PathType
	alloc      stores.SectorFileType
	allowFetch bool
}

func newExistingSelector(index stores.SectorIndex, sector abi.SectorID, exist stores.SectorFileType, alloc stores.SectorFileType, ptype stores.PathType, allowFetch bool) *existingSelector {
	return &existingSelector{
		index:      index,
		sector:     sector,
		exist:      exist,
		ptype:      ptype,
		alloc:      alloc,
		allowFetch: allowFetch,
	}
}

func (s *existingSelector) Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, whnd *workerHandle) (bool, error) {
	tasks := whnd.acceptTasks
	if _, supported := tasks[task]; !supported {
		return false, nil
	}

	paths := whnd.path

	have := map[stores.ID]struct{}{}
	for _, path := range paths {
		have[path.ID] = struct{}{}
	}

	if s.alloc != stores.FTNone {
		best, err := s.index.StorageBestAlloc(ctx, s.alloc, spt, s.ptype)
		if err != nil {
			return false, xerrors.Errorf("finding best alloc storage: %w", err)
		}
		allocOk := false
		for _, info := range best {
			if _, ok := have[info.ID]; ok {
				allocOk = true
			}
		}
		if !allocOk {
			return false, nil
		}
	}
	best, err := s.index.StorageFindSector(ctx, s.sector, s.exist, spt, s.allowFetch)
	if err != nil {
		return false, xerrors.Errorf("finding best storage: %w", err)
	}

	for _, info := range best {
		if _, ok := have[info.ID]; ok {
			return true, nil
		}
	}

	return false, nil
}

func (s *existingSelector) Cmp(ctx context.Context, task sealtasks.TaskType, a, b *workerHandle) (bool, error) {
	return a.utilization() < b.utilization(), nil
}

var _ WorkerSelector = &existingSelector{}
