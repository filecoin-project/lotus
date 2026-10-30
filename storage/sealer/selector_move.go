package sealer

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type moveSelector struct {
	index       paths.SectorIndex
	sector      abi.SectorID
	alloc       storiface.SectorFileType
	destPtype   storiface.PathType
	miner       abi.ActorID
	allowRemote bool
}

func newMoveSelector(index paths.SectorIndex, sector abi.SectorID, alloc storiface.SectorFileType, destPtype storiface.PathType, miner abi.ActorID, allowRemote bool) *moveSelector {
	return &moveSelector{
		index:       index,
		sector:      sector,
		alloc:       alloc,
		destPtype:   destPtype,
		allowRemote: allowRemote,
	}
}

func (s *moveSelector) Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, whnd SchedWorker) (bool, bool, error) {
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

	workerPaths := map[storiface.ID]struct{}{}
	for _, path := range paths {
		workerPaths[path.ID] = struct{}{}
	}

	ssize, err := spt.SectorSize()
	if err != nil {
		return false, false, xerrors.Errorf("getting sector size: %w", err)
	}

	// if the worker already has all files in destination paths, the move is a
	// no-op and must not depend on free space in the destination paths
	inDest := storiface.FTNone
	for _, ft := range s.alloc.AllSet() {
		found, err := s.index.StorageFindSector(ctx, s.sector, ft, ssize, false)
		if err != nil {
			return false, false, xerrors.Errorf("finding sector files: %w", err)
		}

		for _, info := range found {
			if _, has := workerPaths[info.ID]; !has || !ft.Allowed(info.AllowTypes, info.DenyTypes) {
				continue
			}
			if (s.destPtype == storiface.PathStorage && info.CanStore) || (s.destPtype == storiface.PathSealing && info.CanSeal) {
				inDest |= ft
			}
		}
	}
	if inDest == s.alloc {
		return true, true, nil
	}

	// note: allowFetch is always false here, because we want to find workers with
	// the sector available locally
	preferred, err := s.index.StorageFindSector(ctx, s.sector, s.alloc, ssize, false)
	if err != nil {
		return false, false, xerrors.Errorf("finding preferred storage: %w", err)
	}

	var local bool
	for _, info := range preferred {
		if _, ok := workerPaths[info.ID]; ok {
			local = true
			break
		}
	}

	best, err := s.index.StorageBestAlloc(ctx, s.alloc, ssize, s.destPtype, s.miner)
	if err != nil {
		return false, false, xerrors.Errorf("finding best dest storage: %w", err)
	}

	var ok bool
	requested := s.alloc

	for _, info := range best {
		if _, has := workerPaths[info.ID]; has {
			ok = true

			requested = requested.SubAllowed(info.AllowTypes, info.DenyTypes)

			// got all paths
			if requested == storiface.FTNone {
				break
			}
		}
	}

	// if the worker has a local path with the sector already in it, and a
	// destination path, prefer that worker; The move operation is either a
	// no-op because the sector is already in the correct path, or a local move.
	pref := ok && local

	return (ok && s.allowRemote) || pref, pref, nil
}

func (s *moveSelector) Cmp(ctx context.Context, task sealtasks.TaskType, a, b SchedWorker) (bool, error) {
	return a.Utilization() < b.Utilization(), nil
}

var _ WorkerSelector = &moveSelector{}
