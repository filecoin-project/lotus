package sectorstorage

import (
	"container/list"
	"context"
	"sort"
	"sync"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/storiface"
)

const mib = 1 << 20

type WorkerAction func(ctx context.Context, w Worker) error

type WorkerSelector interface {
	Ok(ctx context.Context, task sealtasks.TaskType, a *workerHandle) (bool, error) // true if worker is acceptable for performing a task

	Cmp(ctx context.Context, task sealtasks.TaskType, a, b *workerHandle) (bool, error) // true if a is preferred over b
}

type scheduler struct {
	spt abi.RegisteredProof

	workersLk  sync.Mutex
	nextWorker WorkerID
	workers    map[WorkerID]*workerHandle

	newWorkers chan *workerHandle
	schedule   chan *workerRequest
	workerFree chan WorkerID
	closing    chan struct{}

	schedQueue *list.List // List[*workerRequest]
}

func newScheduler(spt abi.RegisteredProof) *scheduler {
	return &scheduler{
		spt: spt,

		nextWorker: 0,
		workers:    map[WorkerID]*workerHandle{},

		newWorkers: make(chan *workerHandle),
		schedule:   make(chan *workerRequest),
		workerFree: make(chan WorkerID),
		closing:    make(chan struct{}),

		schedQueue: list.New(),
	}
}

func (sh *scheduler) Schedule(ctx context.Context, taskType sealtasks.TaskType, sel WorkerSelector, prepare WorkerAction, work WorkerAction) error {
	ret := make(chan workerResponse)

	select {
	case sh.schedule <- &workerRequest{
		taskType: taskType,
		sel:      sel,

		prepare: prepare,
		work:    work,

		ret: ret,
		ctx: ctx,
	}:
	case <-sh.closing:
		return xerrors.New("closing")
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case resp := <-ret:
		return resp.err
	case <-sh.closing:
		return xerrors.New("closing")
	case <-ctx.Done():
		return ctx.Err()
	}
}

type workerRequest struct {
	taskType sealtasks.TaskType
	sel      WorkerSelector

	prepare WorkerAction
	work    WorkerAction

	ret chan<- workerResponse
	ctx context.Context
}

type workerResponse struct {
	err error
}

func (r *workerRequest) respond(err error) {
	select {
	case r.ret <- workerResponse{err: err}:
	case <-r.ctx.Done():
		log.Warnf("request got cancelled before we could respond")
	}
}

type activeResources struct {
	memUsedMin uint64
	memUsedMax uint64
	gpuUsed    bool
	cpuUse     uint64

	cond *sync.Cond
}

type workerHandle struct {
	w Worker

	info storiface.WorkerInfo

	preparing *activeResources
	active    *activeResources
}

func (sh *scheduler) runSched() {
	for {
		select {
		case w := <-sh.newWorkers:
			sh.schedNewWorker(w)
		case req := <-sh.schedule:
			scheduled, err := sh.maybeSchedRequest(req)
			if err != nil {
				req.respond(err)
				continue
			}
			if scheduled {
				continue
			}

			sh.schedQueue.PushBack(req)
		case wid := <-sh.workerFree:
			sh.onWorkerFreed(wid)
		case <-sh.closing:
			sh.schedClose()
			return
		}
	}
}

func (sh *scheduler) onWorkerFreed(wid WorkerID) {
	for e := sh.schedQueue.Front(); e != nil; e = e.Next() {
		req := e.Value.(*workerRequest)

		ok, err := req.sel.Ok(req.ctx, req.taskType, sh.workers[wid])
		if err != nil {
			log.Errorf("onWorkerFreed req.sel.Ok error: %+v", err)
			continue
		}

		if !ok {
			continue
		}

		scheduled, err := sh.maybeSchedRequest(req)
		if err != nil {
			req.respond(err)
			continue
		}

		if scheduled {
			pe := e.Prev()
			sh.schedQueue.Remove(e)
			if pe == nil {
				pe = sh.schedQueue.Front()
			}
			if pe == nil {
				break
			}
			e = pe
			continue
		}
	}
}

func (sh *scheduler) maybeSchedRequest(req *workerRequest) (bool, error) {
	sh.workersLk.Lock()
	defer sh.workersLk.Unlock()

	tried := 0
	var acceptable []WorkerID

	for wid, worker := range sh.workers {
		ok, err := req.sel.Ok(req.ctx, req.taskType, worker)
		if err != nil {
			return false, err
		}

		if !ok {
			continue
		}
		tried++

		needRes := ResourceTable[req.taskType][sh.spt]
		if !canHandleRequest(needRes, sh.spt, wid, worker.info.Resources, worker.preparing) {
			continue
		}

		acceptable = append(acceptable, wid)
	}

	if len(acceptable) > 0 {
		{
			var serr error

			sort.SliceStable(acceptable, func(i, j int) bool {
				r, err := req.sel.Cmp(req.ctx, req.taskType, sh.workers[acceptable[i]], sh.workers[acceptable[j]])
				if err != nil {
					serr = multierror.Append(serr, err)
				}
				return r
			})

			if serr != nil {
				return false, xerrors.Errorf("error(s) selecting best worker: %w", serr)
			}
		}

		return true, sh.assignWorker(acceptable[0], sh.workers[acceptable[0]], req)
	}

	if tried == 0 {
		return false, xerrors.New("maybeSchedRequest didn't find any good workers")
	}

	return false, nil // put in waiting queue
}

func (sh *scheduler) assignWorker(wid WorkerID, w *workerHandle, req *workerRequest) error {
	needRes := ResourceTable[req.taskType][sh.spt]

	w.preparing.add(w.info.Resources, needRes)

	go func() {
		err := req.prepare(req.ctx, w.w)
		sh.workersLk.Lock()

		if err != nil {
			w.preparing.free(w.info.Resources, needRes)
			sh.workersLk.Unlock()

			select {
			case sh.workerFree <- wid:
			case <-sh.closing:
				log.Warnf("scheduler closed while sending response (prepare error: %+v)", err)
			}

			select {
			case req.ret <- workerResponse{err: err}:
			case <-req.ctx.Done():
				log.Warnf("request got cancelled before we could respond (prepare error: %+v)", err)
			case <-sh.closing:
				log.Warnf("scheduler closed while sending response (prepare error: %+v)", err)
			}
			return
		}

		err = w.active.withResources(sh.spt, wid, w.info.Resources, needRes, &sh.workersLk, func() error {
			w.preparing.free(w.info.Resources, needRes)
			sh.workersLk.Unlock()
			defer sh.workersLk.Lock() // we MUST return locked from this function

			select {
			case sh.workerFree <- wid:
			case <-sh.closing:
			}

			err = req.work(req.ctx, w.w)

			select {
			case req.ret <- workerResponse{err: err}:
			case <-req.ctx.Done():
				log.Warnf("request got cancelled before we could respond")
			case <-sh.closing:
				log.Warnf("scheduler closed while sending response")
			}

			return nil
		})

		sh.workersLk.Unlock()

		// This error should always be nil, since nothing is setting it, but just to be safe:
		if err != nil {
			log.Errorf("error executing worker (withResources): %+v", err)
		}
	}()

	return nil
}

func (a *activeResources) withResources(spt abi.RegisteredProof, id WorkerID, wr storiface.WorkerResources, r Resources, locker sync.Locker, cb func() error) error {
	for !canHandleRequest(r, spt, id, wr, a) {
		if a.cond == nil {
			a.cond = sync.NewCond(locker)
		}
		a.cond.Wait()
	}

	a.add(wr, r)

	err := cb()

	a.free(wr, r)
	if a.cond != nil {
		a.cond.Broadcast()
	}

	return err
}

func (a *activeResources) add(wr storiface.WorkerResources, r Resources) {
	a.gpuUsed = r.CanGPU
	if r.MultiThread() {
		a.cpuUse += wr.CPUs
	} else {
		a.cpuUse += uint64(r.Threads)
	}

	a.memUsedMin += r.MinMemory
	a.memUsedMax += r.MaxMemory
}

func (a *activeResources) free(wr storiface.WorkerResources, r Resources) {
	if r.CanGPU {
		a.gpuUsed = false
	}
	if r.MultiThread() {
		a.cpuUse -= wr.CPUs
	} else {
		a.cpuUse -= uint64(r.Threads)
	}

	a.memUsedMin -= r.MinMemory
	a.memUsedMax -= r.MaxMemory
}

func canHandleRequest(needRes Resources, spt abi.RegisteredProof, wid WorkerID, res storiface.WorkerResources, active *activeResources) bool {

	// TODO: dedupe needRes.BaseMinMemory per task type (don't add if that task is already running)
	minNeedMem := res.MemReserved + active.memUsedMin + needRes.MinMemory + needRes.BaseMinMemory
	if minNeedMem > res.MemPhysical {
		log.Debugf("sched: not scheduling on worker %d; not enough physical memory - need: %dM, have %dM", wid, minNeedMem/mib, res.MemPhysical/mib)
		return false
	}

	maxNeedMem := res.MemReserved + active.memUsedMax + needRes.MaxMemory + needRes.BaseMinMemory
	if spt == abi.RegisteredProof_StackedDRG32GiBSeal {
		maxNeedMem += MaxCachingOverhead
	}
	if maxNeedMem > res.MemSwap+res.MemPhysical {
		log.Debugf("sched: not scheduling on worker %d; not enough virtual memory - need: %dM, have %dM", wid, maxNeedMem/mib, (res.MemSwap+res.MemPhysical)/mib)
		return false
	}

	if needRes.MultiThread() {
		if active.cpuUse > 0 {
			log.Debugf("sched: not scheduling on worker %d; multicore process needs %d threads, %d in use, target %d", wid, res.CPUs, active.cpuUse, res.CPUs)
			return false
		}
	}

	if len(res.GPUs) > 0 && needRes.CanGPU {
		if active.gpuUsed {
			log.Debugf("sched: not scheduling on worker %d; GPU in use", wid)
			return false
		}
	}

	return true
}

func (sh *scheduler) schedNewWorker(w *workerHandle) {
	sh.workersLk.Lock()
	defer sh.workersLk.Unlock()

	id := sh.nextWorker
	sh.workers[id] = w
	sh.nextWorker++
}

func (sh *scheduler) schedClose() {
	sh.workersLk.Lock()
	defer sh.workersLk.Unlock()

	for i, w := range sh.workers {
		if err := w.w.Close(); err != nil {
			log.Errorf("closing worker %d: %+v", i, err)
		}
	}
}

func (sh *scheduler) Close() error {
	close(sh.closing)
	return nil
}
