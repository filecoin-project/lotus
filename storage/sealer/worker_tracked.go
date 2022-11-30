package sealer

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type trackedWork struct {
	job            storiface.WorkerJob
	worker         storiface.WorkerID
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

func (wt *workTracker) track(ctx context.Context, ready chan struct{}, wid storiface.WorkerID, wi storiface.WorkerInfo, sid storiface.SectorRef, task sealtasks.TaskType, cb func() (storiface.CallID, error)) (storiface.CallID, error) {
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

	wt.lk.Unlock()
	callID, err := cb()
	wt.lk.Lock()
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

func (wt *workTracker) worker(wid storiface.WorkerID, wi storiface.WorkerInfo, w Worker) *trackedWorker {
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
	wid        storiface.WorkerID
	workerInfo storiface.WorkerInfo

	execute chan struct{} // channel blocking execution in case we're waiting for resources but the task is ready to execute

	tracker *workTracker
}

func (t *trackedWorker) start() {
	close(t.execute)
}

func (t *trackedWorker) SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTPreCommit1, func() (storiface.CallID, error) { return t.Worker.SealPreCommit1(ctx, sector, ticket, pieces) })
}

func (t *trackedWorker) SealPreCommit2(ctx context.Context, sector storiface.SectorRef, pc1o storiface.PreCommit1Out) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTPreCommit2, func() (storiface.CallID, error) { return t.Worker.SealPreCommit2(ctx, sector, pc1o) })
}

func (t *trackedWorker) SealCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTCommit1, func() (storiface.CallID, error) {
		return t.Worker.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
	})
}

func (t *trackedWorker) SealCommit2(ctx context.Context, sector storiface.SectorRef, c1o storiface.Commit1Out) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTCommit2, func() (storiface.CallID, error) { return t.Worker.SealCommit2(ctx, sector, c1o) })
}

func (t *trackedWorker) FinalizeSector(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTFinalize, func() (storiface.CallID, error) { return t.Worker.FinalizeSector(ctx, sector) })
}

func (t *trackedWorker) ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTFinalizeUnsealed, func() (storiface.CallID, error) { return t.Worker.ReleaseUnsealed(ctx, sector, keepUnsealed) })
}

func (t *trackedWorker) DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, storiface.NoSectorRef, sealtasks.TTDataCid, func() (storiface.CallID, error) {
		return t.Worker.DataCid(ctx, pieceSize, pieceData)
	})
}

func (t *trackedWorker) AddPiece(ctx context.Context, sector storiface.SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTAddPiece, func() (storiface.CallID, error) {
		return t.Worker.AddPiece(ctx, sector, pieceSizes, newPieceSize, pieceData)
	})
}

func (t *trackedWorker) Fetch(ctx context.Context, s storiface.SectorRef, ft storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, s, sealtasks.TTFetch, func() (storiface.CallID, error) { return t.Worker.Fetch(ctx, s, ft, ptype, am) })
}

func (t *trackedWorker) UnsealPiece(ctx context.Context, id storiface.SectorRef, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, id, sealtasks.TTUnseal, func() (storiface.CallID, error) { return t.Worker.UnsealPiece(ctx, id, index, size, randomness, cid) })
}

func (t *trackedWorker) ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTReplicaUpdate, func() (storiface.CallID, error) {
		return t.Worker.ReplicaUpdate(ctx, sector, pieces)
	})
}

func (t *trackedWorker) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTProveReplicaUpdate1, func() (storiface.CallID, error) {
		return t.Worker.ProveReplicaUpdate1(ctx, sector, sectorKey, newSealed, newUnsealed)
	})
}

func (t *trackedWorker) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTProveReplicaUpdate2, func() (storiface.CallID, error) {
		return t.Worker.ProveReplicaUpdate2(ctx, sector, sectorKey, newSealed, newUnsealed, vanillaProofs)
	})
}

func (t *trackedWorker) FinalizeReplicaUpdate(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	return t.tracker.track(ctx, t.execute, t.wid, t.workerInfo, sector, sealtasks.TTFinalizeReplicaUpdate, func() (storiface.CallID, error) { return t.Worker.FinalizeReplicaUpdate(ctx, sector) })
}

var _ Worker = &trackedWorker{}
