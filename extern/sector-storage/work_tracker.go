package sectorstorage

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type workTracker struct {
	lk sync.Mutex

	ctr     uint64
	running map[uint64]storiface.WorkerJob

	// TODO: done, aggregate stats, queue stats, scheduler feedback
}

func (wt *workTracker) track(sid abi.SectorID, task sealtasks.TaskType) func() {
	wt.lk.Lock()
	defer wt.lk.Unlock()

	id := wt.ctr
	wt.ctr++

	wt.running[id] = storiface.WorkerJob{
		ID:     id,
		Sector: sid,
		Task:   task,
		Start:  time.Now(),
	}

	return func() {
		wt.lk.Lock()
		defer wt.lk.Unlock()

		delete(wt.running, id)
	}
}

func (wt *workTracker) worker(w Worker) Worker {
	return &trackedWorker{
		Worker:  w,
		tracker: wt,
	}
}

func (wt *workTracker) Running() []storiface.WorkerJob {
	wt.lk.Lock()
	defer wt.lk.Unlock()

	out := make([]storiface.WorkerJob, 0, len(wt.running))
	for _, job := range wt.running {
		out = append(out, job)
	}

	return out
}

type trackedWorker struct {
	Worker

	tracker *workTracker
}

func (t *trackedWorker) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storage.PreCommit1Out, error) {
	defer t.tracker.track(sector, sealtasks.TTPreCommit1)()

	return t.Worker.SealPreCommit1(ctx, sector, ticket, pieces)
}

func (t *trackedWorker) SealPreCommit2(ctx context.Context, sector abi.SectorID, pc1o storage.PreCommit1Out) (storage.SectorCids, error) {
	defer t.tracker.track(sector, sealtasks.TTPreCommit2)()

	return t.Worker.SealPreCommit2(ctx, sector, pc1o)
}

func (t *trackedWorker) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storage.Commit1Out, error) {
	defer t.tracker.track(sector, sealtasks.TTCommit1)()

	return t.Worker.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
}

func (t *trackedWorker) SealCommit2(ctx context.Context, sector abi.SectorID, c1o storage.Commit1Out) (storage.Proof, error) {
	defer t.tracker.track(sector, sealtasks.TTCommit2)()

	return t.Worker.SealCommit2(ctx, sector, c1o)
}

func (t *trackedWorker) FinalizeSector(ctx context.Context, sector abi.SectorID, keepUnsealed []storage.Range) error {
	defer t.tracker.track(sector, sealtasks.TTFinalize)()

	return t.Worker.FinalizeSector(ctx, sector, keepUnsealed)
}

func (t *trackedWorker) AddPiece(ctx context.Context, sector abi.SectorID, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage.Data) (abi.PieceInfo, error) {
	defer t.tracker.track(sector, sealtasks.TTAddPiece)()

	return t.Worker.AddPiece(ctx, sector, pieceSizes, newPieceSize, pieceData)
}

func (t *trackedWorker) Fetch(ctx context.Context, s abi.SectorID, ft stores.SectorFileType, ptype stores.PathType, am stores.AcquireMode) error {
	defer t.tracker.track(s, sealtasks.TTFetch)()

	return t.Worker.Fetch(ctx, s, ft, ptype, am)
}

func (t *trackedWorker) UnsealPiece(ctx context.Context, id abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) error {
	defer t.tracker.track(id, sealtasks.TTUnseal)()

	return t.Worker.UnsealPiece(ctx, id, index, size, randomness, cid)
}

func (t *trackedWorker) ReadPiece(ctx context.Context, writer io.Writer, id abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	defer t.tracker.track(id, sealtasks.TTReadUnsealed)()

	return t.Worker.ReadPiece(ctx, writer, id, index, size)
}

var _ Worker = &trackedWorker{}
