package sealer

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type existingSelector struct {
	index      paths.SectorIndex
	sector     abi.SectorID
	fileType   storiface.SectorFileType
	miner      abi.ActorID
	allowFetch bool
}

func newExistingSelector(index paths.SectorIndex, sector abi.SectorID, alloc storiface.SectorFileType, miner abi.ActorID, allowFetch bool) *existingSelector {
	return &existingSelector{
		index:      index,
		sector:     sector,
		fileType:   alloc,
		miner:      miner,
		allowFetch: allowFetch,
	}
}

func (s *existingSelector) Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, whnd SchedWorker) (bool, bool, error) {
	tasks, err := whnd.TaskTypes(ctx)
	if err != nil {
		return false, false, xerrors.Errorf("getting supported worker task types: %w", err)
	}
	if _, supported := tasks[task]; !supported {
		return false, false, nil
	}

	paths, err := whnd.Paths(ctx)
	if err != nil {
		return false, false, xerrors.Errorf("getting worker paths: %w", err)
	}

	have := map[storiface.ID]struct{}{}
	for _, path := range paths {
		have[path.ID] = struct{}{}
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return false, false, xerrors.Errorf("getting sector size: %w", err)
	}

	best, err := s.index.StorageFindSector(ctx, s.sector, s.fileType, ssize, s.allowFetch)
	if err != nil {
		return false, false, xerrors.Errorf("finding best storage: %w", err)
	}

	requested := s.fileType

	for _, info := range best {
		if _, ok := have[info.ID]; ok {
			// we're not putting new sector files anywhere
			if !s.allowFetch {
				return true, false, nil
			}

			requested = requested.SubAllowed(info.AllowTypes, info.DenyTypes)

			// got all paths
			if requested == storiface.FTNone {
				break
			}
		}
	}

	return requested == storiface.FTNone, false, nil
}

func (s *existingSelector) Cmp(ctx context.Context, task sealtasks.TaskType, a, b SchedWorker) (bool, error) {
	return a.Utilization() < b.Utilization(), nil
}

var _ WorkerSelector = &existingSelector{}
