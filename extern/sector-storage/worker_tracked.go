package sectorstorage

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/metrics"
)

type trackedWork struct {
	job            storiface.WorkerJob
	worker         WorkerID
	workerHostname string
}

type workTracker struct {
	lk sync.Mutex

	done     map[storiface.CallID]struct{}
	running  map[storiface.CallID]trackedWork
	prepared map[uuid.UUID]trackedWork

	// TODO: done, aggregate stats, queue stats, scheduler feedback
}

func (wt *workTracker) onDone(ctx context.Context, callID storiface.CallID) {
	wt.lk.Lock()
	defer wt.lk.Unlock()

	t, ok := wt.running[callID]
	if !ok {
		wt.done[callID] = struct{}{}

		stats.Record(ctx, metrics.WorkerUntrackedCallsReturned.M(1))
		return
	}

	took := metrics.SinceInMilliseconds(t.job.Start)

	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.TaskType, string(t.job.Task)),
		tag.Upsert(metrics.WorkerHostname, t.workerHostname),
	)
	stats.Record(ctx, metrics.WorkerCallsReturnedCount.M(1), metrics.WorkerCallsReturnedDuration.M(took))

	delete(wt.running, callID)
}

func (wt *workTracker) track(ctx context.Context, ready chan struct{}, wid WorkerID, wi storiface.WorkerInfo, sid storage.SectorRef, task sealtasks.TaskType, cb func() (storiface.CallID, error)) (storiface.CallID, error) {
	tracked := func(rw int, callID storiface.CallID) trackedWork {
		return trackedWork{
			job: storiface.WorkerJob{
				ID:      callID,
				Sector:  sid.ID,
				Task:    task,
				Start:   time.Now(),
				RunWait: rw,
			},
			worker:         wid,
			workerHostname: wi.Hostname,
		}
	}

	wt.lk.Lock()
	defer wt.lk.Unlock()

	select {
	case <-ready:
	case <-ctx.Done():
		return storiface.UndefCall, ctx.Err()
	default:
		prepID := uuid.New()

		wt.prepared[prepID] = tracked(storiface.RWPrepared, storiface.UndefCall)

		wt.lk.Unlock()

		select {
		case <-ready:
		case <-ctx.Done():
			wt.lk.Lock()
			delete(wt.prepared, prepID)
			return storiface.UndefCall, ctx.Err()
		}

		wt.lk.Lock()
		delete(wt.prepared, prepID)
	}

	callID, err := cb()
	if err != nil {
		return callID, err
	}

	_, done := wt.done[callID]
	if done {
		delete(wt.done, callID)
		return callID, err
	}

	wt.running[callID] = tracked(storiface.RWRunning, callID)

	ctx, _ = tag.New(
		ctx,
		tag.Upsert(metrics.TaskType, string(task)),
		tag.Upsert(metrics.WorkerHostname, wi.Hostname),
	)
	stats.Record(ctx, metrics.WorkerCallsStarted.M(1))

	return callID, err
}

func (wt *workTracker) worker(wid WorkerID, wi storiface.WorkerInfo, w Worker) *trackedWorker {
	return &trackedWorker{
		Worker:     w,
		wid:        wid,
		workerInfo: wi,

		execute: make(chan struct{}),

		tracker: wt,
	}
}

func (wt *workTracker) Running() ([]trackedWork, []trackedWork) {
	wt.lk.Lock()
	defer wt.lk.Unlock()

	running := make([]trackedWork, 0, len(wt.running))
	for _, job := range wt.running {
		running = append(running, job)
	}
	prepared := make([]trackedWork, 0, len(wt.prepared))
	for _, job := range wt.prepared {
		prepared = append(prepared, job)
	}

	return running, prepared
}

type trackedWorker struct {
	Worker
	wid        WorkerID
	workerInfo storiface.WorkerInfo

	execute chan struct{} // channel blocking execution in case we're waiting for resources but the task is ready to execute

	tracker *workTracker
}

func (t *trackedWorker) start() {
	close(t.execute)
}

func (t *trackedWorker) SealPreCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTPreCommit1, func() (storiface.CallID, error) { return t.Worker.SealPreCommit1(ctx, sector, ticket, pieces) })
}

func (t *trackedWorker) SealPreCommit2(ctx context.Context, sector storage.SectorRef, pc1o storage.PreCommit1Out) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTPreCommit2, func() (storiface.CallID, error) { return t.Worker.SealPreCommit2(ctx, sector, pc1o) })
}

func (t *trackedWorker) SealCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTCommit1, func() (storiface.CallID, error) { return t.Worker.SealCommit1(ctx, sector, ticket, seed, pieces, cids) })
}

func (t *trackedWorker) SealCommit2(ctx context.Context, sector storage.SectorRef, c1o storage.Commit1Out) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTCommit2, func() (storiface.CallID, error) { return t.Worker.SealCommit2(ctx, sector, c1o) })
}

func (t *trackedWorker) FinalizeSector(ctx context.Context, sector storage.SectorRef, keepUnsealed []storage.Range) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTFinalize, func() (storiface.CallID, error) { return t.Worker.FinalizeSector(ctx, sector, keepUnsealed) })
}

func (t *trackedWorker) AddPiece(ctx context.Context, sector storage.SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage.Data) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTAddPiece, func() (storiface.CallID, error) {
		return t.Worker.AddPiece(ctx, sector, pieceSizes, newPieceSize, pieceData)
	})
}

func (t *trackedWorker) Fetch(ctx context.Context, s storage.SectorRef, ft storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, s, sealtasks.TTFetch, func() (storiface.CallID, error) { return t.Worker.Fetch(ctx, s, ft, ptype, am) })
}

func (t *trackedWorker) UnsealPiece(ctx context.Context, id storage.SectorRef, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, id, sealtasks.TTUnseal, func() (storiface.CallID, error) { return t.Worker.UnsealPiece(ctx, id, index, size, randomness, cid) })
}

var _ Worker = &trackedWorker{}
