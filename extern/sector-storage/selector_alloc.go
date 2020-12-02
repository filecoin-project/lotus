package sectorstorage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type allocSelector struct {
	index stores.SectorIndex
	alloc storiface.SectorFileType
	ptype storiface.PathType
}

func newAllocSelector(index stores.SectorIndex, alloc storiface.SectorFileType, ptype storiface.PathType) *allocSelector {
	return &allocSelector{
		index: index,
		alloc: alloc,
		ptype: ptype,
	}
}

func (s *allocSelector) Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, whnd *workerHandle) (bool, error) {
	tasks, err := whnd.workerRpc.TaskTypes(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting supported worker task types: %w", err)
	}
	if _, supported := tasks[task]; !supported {
		return false, nil
	}

	paths, err := whnd.workerRpc.Paths(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting worker paths: %w", err)
	}

	have := map[stores.ID]struct{}{}
	for _, path := range paths {
		have[path.ID] = struct{}{}
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return false, xerrors.Errorf("getting sector size: %w", err)
	}

	best, err := s.index.StorageBestAlloc(ctx, s.alloc, ssize, s.ptype)
	if err != nil {
		return false, xerrors.Errorf("finding best alloc storage: %w", err)
	}

	for _, info := range best {
		if _, ok := have[info.ID]; ok {
			return true, nil
		}
	}

	return false, nil
}

func (s *allocSelector) Cmp(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, a, b *workerHandle) (bool, error) {
	if s.ptype == storiface.PathSealing {
		return a.utilization() < b.utilization(), nil
	} else {
		return s.storageCmp(ctx, spt, a, b)
	}
}

func (s *allocSelector) storageCmp(ctx context.Context, spt abi.RegisteredSealProof, a, b *workerHandle) (bool, error) {
	aPaths, err := a.workerRpc.Paths(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting worker paths: %w", err)
	}
	aHave := map[stores.ID]struct{}{}
	for _, path := range aPaths {
		aHave[path.ID] = struct{}{}
	}

	bPaths, err := b.workerRpc.Paths(ctx)
	if err != nil {
		return true, xerrors.Errorf("getting worker paths: %w", err)
	}
	bHave := map[stores.ID]struct{}{}
	for _, path := range bPaths {
		bHave[path.ID] = struct{}{}
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return true, xerrors.Errorf("getting sector size: %w", err)
	}
	best, err := s.index.StorageBestAlloc(ctx, s.alloc, ssize, s.ptype)
	if err != nil {
		return true, xerrors.Errorf("finding best alloc storage: %w", err)
	}

	for _, info := range best {
		if _, ok := aHave[info.ID]; ok {
			return true, nil
		}
		if _, ok := bHave[info.ID]; ok {
			return false, nil
		}
	}
	return true, nil
}

var _ WorkerSelector = &allocSelector{}
