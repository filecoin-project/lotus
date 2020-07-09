package sectorstorage

import (
	"container/heap"
	"context"
	"math/rand"
	"sort"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/storiface"
)

type schedPrioCtxKey int

var SchedPriorityKey schedPrioCtxKey
var DefaultSchedPriority = 0
var SelectorTimeout = 5 * time.Second

var (
	SchedWindows = 2
)

func getPriority(ctx context.Context) int {
	sp := ctx.Value(SchedPriorityKey)
	if p, ok := sp.(int); ok {
		return p
	}

	return DefaultSchedPriority
}

func WithPriority(ctx context.Context, priority int) context.Context {
	return context.WithValue(ctx, SchedPriorityKey, priority)
}

const mib = 1 << 20

type WorkerAction func(ctx context.Context, w Worker) error

type WorkerSelector interface {
	Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, a *workerHandle) (bool, error) // true if worker is acceptable for performing a task

	Cmp(ctx context.Context, task sealtasks.TaskType, a, b *workerHandle) (bool, error) // true if a is preferred over b
}

type scheduler struct {
	spt abi.RegisteredSealProof

	workersLk  sync.Mutex
	nextWorker WorkerID
	workers    map[WorkerID]*workerHandle

	newWorkers chan *workerHandle

	watchClosing  chan WorkerID
	workerClosing chan WorkerID

	schedule       chan *workerRequest
	windowRequests chan *schedWindowRequest

	// owned by the sh.runSched goroutine
	schedQueue  *requestQueue
	openWindows []*schedWindowRequest

	closing chan struct{}
}

type workerHandle struct {
	w Worker

	info storiface.WorkerInfo

	preparing *activeResources
	active    *activeResources
}

type schedWindowRequest struct {
	worker WorkerID

	done chan *schedWindow
}

type schedWindow struct {
	worker    WorkerID
	allocated *activeResources
	todo      []*workerRequest
}

type activeResources struct {
	memUsedMin uint64
	memUsedMax uint64
	gpuUsed    bool
	cpuUse     uint64

	cond *sync.Cond
}

type workerRequest struct {
	sector   abi.SectorID
	taskType sealtasks.TaskType
	priority int // larger values more important
	sel      WorkerSelector

	prepare WorkerAction
	work    WorkerAction

	index int // The index of the item in the heap.

	ret chan<- workerResponse
	ctx context.Context
}

type workerResponse struct {
	err error
}

func newScheduler(spt abi.RegisteredSealProof) *scheduler {
	return &scheduler{
		spt: spt,

		nextWorker: 0,
		workers:    map[WorkerID]*workerHandle{},

		newWorkers: make(chan *workerHandle),

		watchClosing:  make(chan WorkerID),
		workerClosing: make(chan WorkerID),

		schedule: make(chan *workerRequest),
		closing:  make(chan struct{}),

		schedQueue: &requestQueue{},
	}
}

func (sh *scheduler) Schedule(ctx context.Context, sector abi.SectorID, taskType sealtasks.TaskType, sel WorkerSelector, prepare WorkerAction, work WorkerAction) error {
	ret := make(chan workerResponse)

	select {
	case sh.schedule <- &workerRequest{
		sector:   sector,
		taskType: taskType,
		priority: getPriority(ctx),
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

func (r *workerRequest) respond(err error) {
	select {
	case r.ret <- workerResponse{err: err}:
	case <-r.ctx.Done():
		log.Warnf("request got cancelled before we could respond")
	}
}

func (sh *scheduler) runSched() {
	go sh.runWorkerWatcher()

	for {
		select {
		case w := <-sh.newWorkers:
			sh.newWorker(w)

		case wid := <-sh.workerClosing:
			sh.dropWorker(wid)

		case req := <-sh.schedule:
			heap.Push(sh.schedQueue, req)
			sh.trySched()

		case req := <-sh.windowRequests:
			sh.openWindows = append(sh.openWindows, req)
			sh.trySched()

		case <-sh.closing:
			sh.schedClose()
			return
		}
	}
}

func (sh *scheduler) trySched() {
	/*
		This assigns tasks to workers based on:
		- Task priority (achieved by handling sh.schedQueue in order, since it's already sorted by priority)
		- Worker resource availability
		- Task-specified worker preference (acceptableWindows array below sorted by this preference)
		- Window request age

		1. For each task in the schedQueue find windows which can handle them
		1.1. Create list of windows capable of handling a task
		1.2. Sort windows according to task selector preferences
		2. Going through schedQueue again, assign task to first acceptable window
		   with resources available
		3. Submit windows with scheduled tasks to workers

	*/

	windows := make([]schedWindow, len(sh.openWindows))
	acceptableWindows := make([][]int, sh.schedQueue.Len())

	// Step 1
	for sqi := 0; sqi < sh.schedQueue.Len(); sqi++ {
		task := (*sh.schedQueue)[sqi]
		needRes := ResourceTable[task.taskType][sh.spt]

		for wnd, windowRequest := range sh.openWindows {
			worker := sh.workers[windowRequest.worker]

			// TODO: allow bigger windows
			if !windows[wnd].allocated.canHandleRequest(needRes, windowRequest.worker, worker.info.Resources) {
				continue
			}

			ok, err := task.sel.Ok(task.ctx, task.taskType, sh.spt, worker)
			if err != nil {
				log.Errorf("trySched(1) req.sel.Ok error: %+v", err)
				continue
			}

			if !ok {
				continue
			}

			acceptableWindows[sqi] = append(acceptableWindows[sqi], wnd)
		}

		if len(acceptableWindows[sqi]) == 0 {
			continue
		}

		// Pick best worker (shuffle in case some workers are equally as good)
		rand.Shuffle(len(acceptableWindows[sqi]), func(i, j int) {
			acceptableWindows[sqi][i], acceptableWindows[sqi][j] = acceptableWindows[sqi][j], acceptableWindows[sqi][i]
		})
		sort.SliceStable(acceptableWindows, func(i, j int) bool {
			wii := sh.openWindows[acceptableWindows[sqi][i]].worker
			wji := sh.openWindows[acceptableWindows[sqi][j]].worker

			if wii == wji {
				// for the same worker prefer older windows
				return acceptableWindows[sqi][i] < acceptableWindows[sqi][j]
			}

			wi := sh.workers[wii]
			wj := sh.workers[wji]

			rpcCtx, cancel := context.WithTimeout(task.ctx, SelectorTimeout)
			defer cancel()

			r, err := task.sel.Cmp(rpcCtx, task.taskType, wi, wj)
			if err != nil {
				log.Error("selecting best worker: %s", err)
			}
			return r
		})
	}

	// Step 2
	scheduled := 0

	for sqi := 0; sqi < sh.schedQueue.Len(); sqi++ {
		task := (*sh.schedQueue)[sqi]
		needRes := ResourceTable[task.taskType][sh.spt]

		selectedWindow := -1
		for _, wnd := range acceptableWindows[sqi+scheduled] {
			wid := sh.openWindows[wnd].worker
			wr := sh.workers[wid].info.Resources

			// TODO: allow bigger windows
			if windows[wnd].allocated.canHandleRequest(needRes, wid, wr) {
				continue
			}

			windows[wnd].allocated.add(wr, needRes)

			selectedWindow = wnd
			break
		}

		windows[selectedWindow].todo = append(windows[selectedWindow].todo, task)

		heap.Remove(sh.schedQueue, sqi)
		sqi--
		scheduled++
	}

	// Step 3

	if scheduled == 0 {
		return
	}

	scheduledWindows := map[int]struct{}{}
	for wnd, window := range windows {
		if len(window.todo) == 0 {
			// Nothing scheduled here, keep the window open
			continue
		}

		scheduledWindows[wnd] = struct{}{}

		select {
		case sh.openWindows[wnd].done <- &window:
		default:
			log.Error("expected sh.openWindows[wnd].done to be buffered")
		}
	}

	// Rewrite sh.openWindows array, removing scheduled windows
	newOpenWindows := make([]*schedWindowRequest, 0, len(sh.openWindows)-len(scheduledWindows))
	for wnd, window := range sh.openWindows {
		if _, scheduled := scheduledWindows[wnd]; !scheduled {
			// keep unscheduled windows open
			continue
		}

		newOpenWindows = append(newOpenWindows, window)
	}

	sh.openWindows = newOpenWindows
}

func (sh *scheduler) runWorker(wid WorkerID) {
	w := sh.workers[wid]

	go func() {
		for {

		}
	}()
}

func (a *activeResources) withResources(id WorkerID, wr storiface.WorkerResources, r Resources, locker sync.Locker, cb func() error) error {
	for !a.canHandleRequest(r, id, wr) {
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

func (a *activeResources) canHandleRequest(needRes Resources, wid WorkerID, res storiface.WorkerResources) bool {

	// TODO: dedupe needRes.BaseMinMemory per task type (don't add if that task is already running)
	minNeedMem := res.MemReserved + a.memUsedMin + needRes.MinMemory + needRes.BaseMinMemory
	if minNeedMem > res.MemPhysical {
		log.Debugf("sched: not scheduling on worker %d; not enough physical memory - need: %dM, have %dM", wid, minNeedMem/mib, res.MemPhysical/mib)
		return false
	}

	maxNeedMem := res.MemReserved + a.memUsedMax + needRes.MaxMemory + needRes.BaseMinMemory

	if maxNeedMem > res.MemSwap+res.MemPhysical {
		log.Debugf("sched: not scheduling on worker %d; not enough virtual memory - need: %dM, have %dM", wid, maxNeedMem/mib, (res.MemSwap+res.MemPhysical)/mib)
		return false
	}

	if needRes.MultiThread() {
		if a.cpuUse > 0 {
			log.Debugf("sched: not scheduling on worker %d; multicore process needs %d threads, %d in use, target %d", wid, res.CPUs, a.cpuUse, res.CPUs)
			return false
		}
	} else {
		if a.cpuUse+uint64(needRes.Threads) > res.CPUs {
			log.Debugf("sched: not scheduling on worker %d; not enough threads, need %d, %d in use, target %d", wid, needRes.Threads, a.cpuUse, res.CPUs)
			return false
		}
	}

	if len(res.GPUs) > 0 && needRes.CanGPU {
		if a.gpuUsed {
			log.Debugf("sched: not scheduling on worker %d; GPU in use", wid)
			return false
		}
	}

	return true
}

func (a *activeResources) utilization(wr storiface.WorkerResources) float64 {
	var max float64

	cpu := float64(a.cpuUse) / float64(wr.CPUs)
	max = cpu

	memMin := float64(a.memUsedMin+wr.MemReserved) / float64(wr.MemPhysical)
	if memMin > max {
		max = memMin
	}

	memMax := float64(a.memUsedMax+wr.MemReserved) / float64(wr.MemPhysical+wr.MemSwap)
	if memMax > max {
		max = memMax
	}

	return max
}

func (sh *scheduler) newWorker(w *workerHandle) {
	sh.workersLk.Lock()

	id := sh.nextWorker
	sh.workers[id] = w
	sh.nextWorker++

	sh.workersLk.Unlock()

	select {
	case sh.watchClosing <- id:
	case <-sh.closing:
		return
	}

	sh.runWorker(id)
}

func (sh *scheduler) dropWorker(wid WorkerID) {
	sh.workersLk.Lock()
	defer sh.workersLk.Unlock()

	w := sh.workers[wid]
	delete(sh.workers, wid)

	go func() {
		if err := w.w.Close(); err != nil {
			log.Warnf("closing worker %d: %+v", err)
		}
	}()
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
