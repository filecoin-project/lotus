package sectorstorage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/stores"
)

type allocSelector struct {
	best []stores.StorageInfo
}

func newAllocSelector(ctx context.Context, index stores.SectorIndex, alloc stores.SectorFileType) (*allocSelector, error) {
	best, err := index.StorageBestAlloc(ctx, alloc, true)
	if err != nil {
		return nil, err
	}

	return &allocSelector{
		best: best,
	}, nil
}

func (s *allocSelector) Ok(ctx context.Context, task sealtasks.TaskType, whnd *workerHandle) (bool, error) {
	tasks, err := whnd.w.TaskTypes(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting supported worker task types: %w", err)
	}
	if _, supported := tasks[task]; !supported {
		return false, nil
	}

	paths, err := whnd.w.Paths(ctx)
	if err != nil {
		return false, xerrors.Errorf("getting worker paths: %w", err)
	}

	have := map[stores.ID]struct{}{}
	for _, path := range paths {
		have[path.ID] = struct{}{}
	}

	for _, info := range s.best {
		if _, ok := have[info.ID]; ok {
			return true, nil
		}
	}

	return false, nil
}

func (s *allocSelector) Cmp(ctx context.Context, task sealtasks.TaskType, a, b *workerHandle) (bool, error) {
	return a.info.Hostname > b.info.Hostname, nil // TODO: Better strategy
}

var _ WorkerSelector = &allocSelector{}
