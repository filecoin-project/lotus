package sealer

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type poStScheduler struct {
	lk      sync.RWMutex
	workers map[storiface.WorkerID]*WorkerHandle
	cond    *sync.Cond

	postType sealtasks.TaskType
}

func newPoStScheduler(t sealtasks.TaskType) *poStScheduler {
	ps := &poStScheduler{
		workers:  map[storiface.WorkerID]*WorkerHandle{},
		postType: t,
	}
	ps.cond = sync.NewCond(&ps.lk)
	return ps
}

func (ps *poStScheduler) MaybeAddWorker(wid storiface.WorkerID, tasks map[sealtasks.TaskType]struct{}, w *WorkerHandle) bool {
	if _, ok := tasks[ps.postType]; !ok {
		return false
	}

	ps.lk.Lock()
	defer ps.lk.Unlock()

	ps.workers[wid] = w

	go ps.watch(wid, w)

	ps.cond.Broadcast()

	return true
}

func (ps *poStScheduler) delWorker(wid storiface.WorkerID) *WorkerHandle {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	var w *WorkerHandle
	if wh, ok := ps.workers[wid]; ok {
		w = wh
		delete(ps.workers, wid)
	}
	return w
}

func (ps *poStScheduler) CanSched(ctx context.Context) bool {
	ps.lk.RLock()
	defer ps.lk.RUnlock()
	if len(ps.workers) == 0 {
		return false
	}

	for _, w := range ps.workers {
		if w.Enabled {
			return true
		}
	}

	return false
}

func (ps *poStScheduler) Schedule(ctx context.Context, primary bool, spt abi.RegisteredSealProof, work WorkerAction) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if len(ps.workers) == 0 {
		return xerrors.Errorf("can't find %s post worker", ps.postType)
	}

	// Get workers by resource
	canDo, candidates := ps.readyWorkers(spt)
	for !canDo {
		//if primary is true, it must be dispatched to a worker
		if primary {
			ps.cond.Wait()
			canDo, candidates = ps.readyWorkers(spt)
		} else {
			return xerrors.Errorf("can't find %s post worker", ps.postType)
		}
	}

	defer func() {
		if ps.cond != nil {
			ps.cond.Broadcast()
		}
	}()

	var rpcErrs error

	for i, selected := range candidates {
		worker := ps.workers[selected.id]

		err := worker.active.withResources(uuid.UUID{}, selected.id, worker.Info, ps.postType.SealTask(spt), selected.res, &ps.lk, func() error {
			ps.lk.Unlock()
			defer ps.lk.Lock()

			return work(ctx, worker.workerRpc)
		})
		if err == nil {
			return nil
		}

		// if the error is RPCConnectionError, try another worker, if not, return the error
		if !errors.As(err, new(*jsonrpc.RPCConnectionError)) {
			return err
		}

		log.Warnw("worker RPC connection error, will retry with another candidate if possible", "error", err, "worker", selected.id, "candidate", i, "candidates", len(candidates))
		rpcErrs = multierror.Append(rpcErrs, err)
	}

	return xerrors.Errorf("got RPC errors from all workers: %w", rpcErrs)
}

type candidateWorker struct {
	id  storiface.WorkerID
	res storiface.Resources
}

func (ps *poStScheduler) readyWorkers(spt abi.RegisteredSealProof) (bool, []candidateWorker) {
	var accepts []candidateWorker
	//if the gpus of the worker are insufficient or it's disabled, it cannot be scheduled
	for wid, wr := range ps.workers {
		needRes := wr.Info.Resources.ResourceSpec(spt, ps.postType)

		if !wr.Enabled {
			log.Debugf("sched: not scheduling on PoSt-worker %s, worker disabled", wid)
			continue
		}

		if !wr.active.CanHandleRequest(uuid.UUID{}, ps.postType.SealTask(spt), needRes, wid, "post-readyWorkers", wr.Info) {
			continue
		}

		accepts = append(accepts, candidateWorker{
			id:  wid,
			res: needRes,
		})
	}

	// todo: round robin or something
	rand.Shuffle(len(accepts), func(i, j int) {
		accepts[i], accepts[j] = accepts[j], accepts[i]
	})

	return len(accepts) != 0, accepts
}

func (ps *poStScheduler) disable(wid storiface.WorkerID) {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	ps.workers[wid].Enabled = false
}

func (ps *poStScheduler) enable(wid storiface.WorkerID) {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	ps.workers[wid].Enabled = true
}

func (ps *poStScheduler) watch(wid storiface.WorkerID, worker *WorkerHandle) {
	heartbeatTimer := time.NewTicker(paths.HeartbeatInterval)
	defer heartbeatTimer.Stop()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	defer close(worker.closedMgr)

	defer func() {
		log.Warnw("Worker closing", "WorkerID", wid)
		ps.delWorker(wid)
	}()

	for {
		select {
		case <-heartbeatTimer.C:
		case <-worker.closingMgr:
			return
		}

		sctx, scancel := context.WithTimeout(ctx, paths.HeartbeatInterval/2)
		curSes, err := worker.workerRpc.Session(sctx)
		scancel()
		if err != nil {
			// Likely temporary error
			log.Warnw("failed to check worker session", "error", err)
			ps.disable(wid)

			continue
		}

		if storiface.WorkerID(curSes) != wid {
			if curSes != ClosedWorkerID {
				// worker restarted
				log.Warnw("worker session changed (worker restarted?)", "initial", wid, "current", curSes)
			}
			return
		}

		ps.enable(wid)
	}
}

func (ps *poStScheduler) workerCleanup(wid storiface.WorkerID, w *WorkerHandle) {
	select {
	case <-w.closingMgr:
	default:
		close(w.closingMgr)
	}

	ps.lk.Unlock()
	select {
	case <-w.closedMgr:
	case <-time.After(time.Second):
		log.Errorf("timeout closing worker manager goroutine %s", wid)
	}
	ps.lk.Lock()
}

func (ps *poStScheduler) schedClose() {
	ps.lk.Lock()
	defer ps.lk.Unlock()
	log.Debugf("closing scheduler")

	for i, w := range ps.workers {
		ps.workerCleanup(i, w)
	}
}

func (ps *poStScheduler) WorkerStats(ctx context.Context, cb func(ctx context.Context, wid storiface.WorkerID, worker *WorkerHandle)) {
	ps.lk.RLock()
	defer ps.lk.RUnlock()
	for id, w := range ps.workers {
		cb(ctx, id, w)
	}
}
