package sealer

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type schedWorker struct {
	sched  *Scheduler
	worker *WorkerHandle

	wid storiface.WorkerID

	heartbeatTimer   *time.Ticker
	scheduledWindows chan *SchedWindow
	taskDone         chan struct{}

	windowsRequested int
}

func newWorkerHandle(ctx context.Context, w Worker) (*WorkerHandle, error) {
	info, err := w.Info(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting worker info: %w", err)
	}

	tc := newTaskCounter()

	worker := &WorkerHandle{
		workerRpc: w,
		Info:      info,

		preparing: NewActiveResources(tc),
		active:    NewActiveResources(tc),
		Enabled:   true,

		closingMgr: make(chan struct{}),
		closedMgr:  make(chan struct{}),
	}

	return worker, nil
}

// context only used for startup
func (sh *Scheduler) runWorker(ctx context.Context, wid storiface.WorkerID, worker *WorkerHandle) error {
	sh.workersLk.Lock()
	_, exist := sh.Workers[wid]
	if exist {
		log.Warnw("duplicated worker added", "id", wid)

		// this is ok, we're already handling this worker in a different goroutine
		sh.workersLk.Unlock()
		return nil
	}

	sh.Workers[wid] = worker
	sh.workersLk.Unlock()

	sw := &schedWorker{
		sched:  sh,
		worker: worker,

		wid: wid,

		heartbeatTimer:   time.NewTicker(paths.HeartbeatInterval),
		scheduledWindows: make(chan *SchedWindow, SchedWindows),
		taskDone:         make(chan struct{}, 1),

		windowsRequested: 0,
	}

	go sw.handleWorker()

	return nil
}

func (sw *schedWorker) handleWorker() {
	worker, sched := sw.worker, sw.sched

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	defer close(worker.closedMgr)

	defer func() {
		log.Warnw("Worker closing", "workerid", sw.wid)

		if err := sw.disable(ctx); err != nil {
			log.Warnw("failed to disable worker", "worker", sw.wid, "error", err)
		}

		sched.workersLk.Lock()
		delete(sched.Workers, sw.wid)
		sched.workersLk.Unlock()
	}()

	defer sw.heartbeatTimer.Stop()

	for {
		{
			sched.workersLk.Lock()
			enabled := worker.Enabled
			sched.workersLk.Unlock()

			// ask for more windows if we need them (non-blocking)
			if enabled {
				if !sw.requestWindows() {
					return // graceful shutdown
				}
			}
		}

		// wait for more windows to come in, or for tasks to get finished (blocking)
		for {
			// ping the worker and check session
			if !sw.checkSession(ctx) {
				return // invalid session / exiting
			}

			// session looks good
			{
				sched.workersLk.Lock()
				enabled := worker.Enabled
				worker.Enabled = true
				sched.workersLk.Unlock()

				if !enabled {
					// go send window requests
					break
				}
			}

			// wait for more tasks to be assigned by the main scheduler or for the worker
			// to finish processing a task
			update, pokeSched, ok := sw.waitForUpdates()
			if !ok {
				return
			}
			if pokeSched {
				// a task has finished preparing, which can mean that we've freed some space on some worker
				select {
				case sched.workerChange <- struct{}{}:
				default: // workerChange is buffered, and scheduling is global, so it's ok if we don't send here
				}
			}
			if update {
				break
			}
		}

		// process assigned windows (non-blocking)
		sched.workersLk.RLock()
		worker.wndLk.Lock()

		sw.workerCompactWindows()

		// send tasks to the worker
		sw.processAssignedWindows()

		worker.wndLk.Unlock()
		sched.workersLk.RUnlock()
	}
}

func (sw *schedWorker) disable(ctx context.Context) error {
	done := make(chan struct{})

	// request cleanup in the main scheduler goroutine
	select {
	case sw.sched.workerDisable <- workerDisableReq{
		activeWindows: sw.worker.activeWindows,
		wid:           sw.wid,
		done: func() {
			close(done)
		},
	}:
	case <-ctx.Done():
		return ctx.Err()
	case <-sw.sched.closing:
		return nil
	}

	// wait for cleanup to complete
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	case <-sw.sched.closing:
		return nil
	}

	sw.worker.activeWindows = sw.worker.activeWindows[:0]
	sw.windowsRequested = 0
	return nil
}

func (sw *schedWorker) checkSession(ctx context.Context) bool {
	for {
		sctx, scancel := context.WithTimeout(ctx, paths.HeartbeatInterval/2)
		curSes, err := sw.worker.workerRpc.Session(sctx)
		scancel()
		if err != nil {
			// Likely temporary error

			log.Warnw("failed to check worker session", "error", err)

			if err := sw.disable(ctx); err != nil {
				log.Warnw("failed to disable worker with session error", "worker", sw.wid, "error", err)
			}

			select {
			case <-sw.heartbeatTimer.C:
				continue
			case w := <-sw.scheduledWindows:
				// was in flight when initially disabled, return
				sw.worker.wndLk.Lock()
				sw.worker.activeWindows = append(sw.worker.activeWindows, w)
				sw.worker.wndLk.Unlock()

				if err := sw.disable(ctx); err != nil {
					log.Warnw("failed to disable worker with session error", "worker", sw.wid, "error", err)
				}
			case <-sw.sched.closing:
				return false
			case <-sw.worker.closingMgr:
				return false
			}
			continue
		}

		if storiface.WorkerID(curSes) != sw.wid {
			if curSes != ClosedWorkerID {
				// worker restarted
				log.Warnw("worker session changed (worker restarted?)", "initial", sw.wid, "current", curSes)
			}

			return false
		}

		return true
	}
}

func (sw *schedWorker) requestWindows() bool {
	for ; sw.windowsRequested < SchedWindows; sw.windowsRequested++ {
		select {
		case sw.sched.windowRequests <- &SchedWindowRequest{
			Worker: sw.wid,
			Done:   sw.scheduledWindows,
		}:
		case <-sw.sched.closing:
			return false
		case <-sw.worker.closingMgr:
			return false
		}
	}
	return true
}

func (sw *schedWorker) waitForUpdates() (update bool, sched bool, ok bool) {
	select {
	case <-sw.heartbeatTimer.C:
		return false, false, true
	case w := <-sw.scheduledWindows:
		sw.worker.wndLk.Lock()
		sw.worker.activeWindows = append(sw.worker.activeWindows, w)
		sw.worker.wndLk.Unlock()
		return true, false, true
	case <-sw.taskDone:
		log.Debugw("task done", "workerid", sw.wid)
		return true, true, true
	case <-sw.sched.closing:
	case <-sw.worker.closingMgr:
	}

	return false, false, false
}

func (sw *schedWorker) workerCompactWindows() {
	worker := sw.worker

	// move tasks from older windows to newer windows if older windows
	// still can fit them
	if len(worker.activeWindows) > 1 {
		for wi, window := range worker.activeWindows[1:] {
			lower := worker.activeWindows[wi]
			var moved []int

			for ti, todo := range window.Todo {
				needRes := worker.Info.Resources.ResourceSpec(todo.Sector.ProofType, todo.TaskType)
				if !lower.Allocated.CanHandleRequest(todo.SchedId, todo.SealTask(), needRes, sw.wid, "compactWindows", worker.Info) {
					continue
				}

				moved = append(moved, ti)
				lower.Todo = append(lower.Todo, todo)
				lower.Allocated.Add(todo.SchedId, todo.SealTask(), worker.Info.Resources, needRes)
				window.Allocated.Free(todo.SchedId, todo.SealTask(), worker.Info.Resources, needRes)
			}

			if len(moved) > 0 {
				newTodo := make([]*WorkerRequest, 0, len(window.Todo)-len(moved))
				for i, t := range window.Todo {
					if len(moved) > 0 && moved[0] == i {
						moved = moved[1:]
						continue
					}

					newTodo = append(newTodo, t)
				}
				window.Todo = newTodo
			}
		}
	}

	var compacted int
	var newWindows []*SchedWindow

	for _, window := range worker.activeWindows {
		if len(window.Todo) == 0 {
			compacted++
			continue
		}

		newWindows = append(newWindows, window)
	}

	worker.activeWindows = newWindows
	sw.windowsRequested -= compacted
}

func (sw *schedWorker) processAssignedWindows() {
	sw.assignReadyWork()
	sw.assignPreparingWork()
}

func (sw *schedWorker) assignPreparingWork() {
	worker := sw.worker

assignLoop:
	// process windows in order
	for len(worker.activeWindows) > 0 {
		firstWindow := worker.activeWindows[0]

		// process tasks within a window, preferring tasks at lower indexes
		for len(firstWindow.Todo) > 0 {
			tidx := -1

			worker.lk.Lock()
			for t, todo := range firstWindow.Todo {
				needResPrep := worker.Info.Resources.PrepResourceSpec(todo.Sector.ProofType, todo.TaskType, todo.prepare.PrepType)
				if worker.preparing.CanHandleRequest(todo.SchedId, todo.PrepSealTask(), needResPrep, sw.wid, "startPreparing", worker.Info) {
					tidx = t
					break
				}
			}
			worker.lk.Unlock()

			if tidx == -1 {
				break assignLoop
			}

			todo := firstWindow.Todo[tidx]

			log.Debugf("assign worker sector %d to %s", todo.Sector.ID.Number, worker.Info.Hostname)
			err := sw.startProcessingTask(todo)

			if err != nil {
				log.Errorf("startProcessingTask error: %+v", err)
				go todo.respond(xerrors.Errorf("startProcessingTask error: %w", err))
			}

			// Note: we're not freeing window.allocated resources here very much on purpose
			copy(firstWindow.Todo[tidx:], firstWindow.Todo[tidx+1:])
			firstWindow.Todo[len(firstWindow.Todo)-1] = nil
			firstWindow.Todo = firstWindow.Todo[:len(firstWindow.Todo)-1]
		}

		copy(worker.activeWindows, worker.activeWindows[1:])
		worker.activeWindows[len(worker.activeWindows)-1] = nil
		worker.activeWindows = worker.activeWindows[:len(worker.activeWindows)-1]

		sw.windowsRequested--
	}
}

func (sw *schedWorker) assignReadyWork() {
	worker := sw.worker

	worker.lk.Lock()
	defer worker.lk.Unlock()

	if worker.active.hasWorkWaiting() {
		// prepared tasks have priority
		return
	}

assignLoop:
	// process windows in order
	for len(worker.activeWindows) > 0 {
		firstWindow := worker.activeWindows[0]

		// process tasks within a window, preferring tasks at lower indexes
		for len(firstWindow.Todo) > 0 {
			tidx := -1

			for t, todo := range firstWindow.Todo {
				if todo.TaskType != sealtasks.TTCommit1 && todo.TaskType != sealtasks.TTCommit2 { // todo put in task
					continue
				}

				needRes := worker.Info.Resources.ResourceSpec(todo.Sector.ProofType, todo.TaskType)
				if worker.active.CanHandleRequest(todo.SchedId, todo.SealTask(), needRes, sw.wid, "startPreparing", worker.Info) {
					tidx = t
					break
				}
			}

			if tidx == -1 {
				break assignLoop
			}

			todo := firstWindow.Todo[tidx]

			log.Debugf("assign worker sector %d (ready)", todo.Sector.ID.Number)
			err := sw.startProcessingReadyTask(todo)

			if err != nil {
				log.Errorf("startProcessingTask error: %+v", err)
				go todo.respond(xerrors.Errorf("startProcessingTask error: %w", err))
			}

			// Note: we're not freeing window.allocated resources here very much on purpose
			copy(firstWindow.Todo[tidx:], firstWindow.Todo[tidx+1:])
			firstWindow.Todo[len(firstWindow.Todo)-1] = nil
			firstWindow.Todo = firstWindow.Todo[:len(firstWindow.Todo)-1]
		}

		copy(worker.activeWindows, worker.activeWindows[1:])
		worker.activeWindows[len(worker.activeWindows)-1] = nil
		worker.activeWindows = worker.activeWindows[:len(worker.activeWindows)-1]

		sw.windowsRequested--
	}
}

func (sw *schedWorker) startProcessingTask(req *WorkerRequest) error {
	w, sh := sw.worker, sw.sched

	needRes := w.Info.Resources.ResourceSpec(req.Sector.ProofType, req.TaskType)
	needResPrep := w.Info.Resources.PrepResourceSpec(req.Sector.ProofType, req.TaskType, req.prepare.PrepType)

	w.lk.Lock()
	w.preparing.Add(req.SchedId, req.PrepSealTask(), w.Info.Resources, needResPrep)
	w.lk.Unlock()

	go func() {
		// first run the prepare step (e.g. fetching sector data from other worker)
		tw := sh.workTracker.worker(sw.wid, w.Info, w.workerRpc)
		tw.start()
		err := req.prepare.Action(req.Ctx, tw)
		w.lk.Lock()

		if err != nil {
			w.preparing.Free(req.SchedId, req.PrepSealTask(), w.Info.Resources, needResPrep)
			w.lk.Unlock()

			select {
			case sw.taskDone <- struct{}{}:
			case <-sh.closing:
				log.Warnf("scheduler closed while sending response (prepare error: %+v)", err)
			default: // there is a notification pending already
			}

			select {
			case req.ret <- workerResponse{err: err}:
			case <-req.Ctx.Done():
				log.Warnf("request got cancelled before we could respond (prepare error: %+v)", err)
			case <-sh.closing:
				log.Warnf("scheduler closed while sending response (prepare error: %+v)", err)
			}
			return
		}

		tw = sh.workTracker.worker(sw.wid, w.Info, w.workerRpc)

		// start tracking work first early in case we need to wait for resources
		werr := make(chan error, 1)
		go func() {
			werr <- req.work(req.Ctx, tw)
		}()

		// wait (if needed) for resources in the 'active' window
		err = w.active.withResources(req.SchedId, sw.wid, w.Info, req.SealTask(), needRes, &w.lk, func() error {
			w.preparing.Free(req.SchedId, req.PrepSealTask(), w.Info.Resources, needResPrep)
			w.lk.Unlock()
			defer w.lk.Lock() // we MUST return locked from this function

			// make sure the worker loop sees that the prepare task has finished
			select {
			case sw.taskDone <- struct{}{}:
			case <-sh.closing:
			default: // there is a notification pending already
			}

			// Do the work!
			tw.start()
			err = <-werr

			select {
			case req.ret <- workerResponse{err: err}:
			case <-req.Ctx.Done():
				log.Warnf("request got cancelled before we could respond")
			case <-sh.closing:
				log.Warnf("scheduler closed while sending response")
			}

			return nil
		})

		w.lk.Unlock()

		// make sure the worker loop sees that the task has finished
		select {
		case sw.taskDone <- struct{}{}:
		default: // there is a notification pending already
		}

		// This error should always be nil, since nothing is setting it, but just to be safe:
		if err != nil {
			log.Errorf("error executing worker (withResources): %+v", err)
		}
	}()

	return nil
}

func (sw *schedWorker) startProcessingReadyTask(req *WorkerRequest) error {
	w, sh := sw.worker, sw.sched

	needRes := w.Info.Resources.ResourceSpec(req.Sector.ProofType, req.TaskType)

	w.active.Add(req.SchedId, req.SealTask(), w.Info.Resources, needRes)

	go func() {
		// Do the work!
		tw := sh.workTracker.worker(sw.wid, w.Info, w.workerRpc)
		tw.start()
		err := req.work(req.Ctx, tw)

		select {
		case req.ret <- workerResponse{err: err}:
		case <-req.Ctx.Done():
			log.Warnf("request got cancelled before we could respond")
		case <-sh.closing:
			log.Warnf("scheduler closed while sending response")
		}

		w.lk.Lock()

		w.active.Free(req.SchedId, req.SealTask(), w.Info.Resources, needRes)

		select {
		case sw.taskDone <- struct{}{}:
		case <-sh.closing:
			log.Warnf("scheduler closed while sending response (prepare error: %+v)", err)
		default: // there is a notification pending already
		}

		w.lk.Unlock()

		// This error should always be nil, since nothing is setting it, but just to be safe:
		if err != nil {
			log.Errorf("error executing worker (ready): %+v", err)
		}
	}()

	return nil
}

func (sh *Scheduler) workerCleanup(wid storiface.WorkerID, w *WorkerHandle) {
	select {
	case <-w.closingMgr:
	default:
		close(w.closingMgr)
	}

	sh.workersLk.Unlock()
	select {
	case <-w.closedMgr:
	case <-time.After(time.Second):
		log.Errorf("timeout closing worker manager goroutine %d", wid)
	}
	sh.workersLk.Lock()

	if !w.cleanupStarted {
		w.cleanupStarted = true

		newWindows := make([]*SchedWindowRequest, 0, len(sh.OpenWindows))
		for _, window := range sh.OpenWindows {
			if window.Worker != wid {
				newWindows = append(newWindows, window)
			}
		}
		sh.OpenWindows = newWindows

		log.Debugf("worker %s dropped", wid)
	}
}
