package sectorstorage

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
)

// context only used for startup
func (sh *scheduler) runWorker(ctx context.Context, w Worker) error {
	info, err := w.Info(ctx)
	if err != nil {
		return xerrors.Errorf("getting worker info: %w", err)
	}

	sessID, err := w.Session(ctx)
	if err != nil {
		return xerrors.Errorf("getting worker session: %w", err)
	}
	if sessID == ClosedWorkerID {
		return xerrors.Errorf("worker already closed")
	}

	worker := &workerHandle{
		w:    w,
		info: info,

		preparing: &activeResources{},
		active:    &activeResources{},
		enabled:   true,

		closingMgr: make(chan struct{}),
		closedMgr:  make(chan struct{}),
	}

	wid := WorkerID(sessID)

	sh.workersLk.Lock()
	_, exist := sh.workers[wid]
	if exist {
		log.Warnw("duplicated worker added", "id", wid)

		// this is ok, we're already handling this worker in a different goroutine
		return nil
	}

	sh.workers[wid] = worker
	sh.workersLk.Unlock()

	go func() {
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		defer close(worker.closedMgr)

		scheduledWindows := make(chan *schedWindow, SchedWindows)
		taskDone := make(chan struct{}, 1)
		windowsRequested := 0

		disable := func(ctx context.Context) error {
			done := make(chan struct{})

			// request cleanup in the main scheduler goroutine
			select {
			case sh.workerDisable <- workerDisableReq{
				activeWindows: worker.activeWindows,
				wid:           wid,
				done: func() {
					close(done)
				},
			}:
			case <-ctx.Done():
				return ctx.Err()
			case <-sh.closing:
				return nil
			}

			// wait for cleanup to complete
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			case <-sh.closing:
				return nil
			}

			worker.activeWindows = worker.activeWindows[:0]
			windowsRequested = 0
			return nil
		}

		defer func() {
			log.Warnw("Worker closing", "workerid", sessID)

			if err := disable(ctx); err != nil {
				log.Warnw("failed to disable worker", "worker", wid, "error", err)
			}

			sh.workersLk.Lock()
			delete(sh.workers, wid)
			sh.workersLk.Unlock()
		}()

		heartbeatTimer := time.NewTicker(stores.HeartbeatInterval)
		defer heartbeatTimer.Stop()

		for {
			sh.workersLk.Lock()
			enabled := worker.enabled
			sh.workersLk.Unlock()

			// ask for more windows if we need them (non-blocking)
			for ; enabled && windowsRequested < SchedWindows; windowsRequested++ {
				select {
				case sh.windowRequests <- &schedWindowRequest{
					worker: wid,
					done:   scheduledWindows,
				}:
				case <-sh.closing:
					return
				case <-worker.closingMgr:
					return
				}
			}

			// wait for more windows to come in, or for tasks to get finished (blocking)
			for {

				// first ping the worker and check session
				{
					sctx, scancel := context.WithTimeout(ctx, stores.HeartbeatInterval/2)
					curSes, err := worker.w.Session(sctx)
					scancel()
					if err != nil {
						// Likely temporary error

						log.Warnw("failed to check worker session", "error", err)

						if err := disable(ctx); err != nil {
							log.Warnw("failed to disable worker with session error", "worker", wid, "error", err)
						}

						select {
						case <-heartbeatTimer.C:
							continue
						case w := <-scheduledWindows:
							// was in flight when initially disabled, return
							worker.wndLk.Lock()
							worker.activeWindows = append(worker.activeWindows, w)
							worker.wndLk.Unlock()

							if err := disable(ctx); err != nil {
								log.Warnw("failed to disable worker with session error", "worker", wid, "error", err)
							}
						case <-sh.closing:
							return
						case <-worker.closingMgr:
							return
						}
						continue
					}

					if curSes != sessID {
						if curSes != ClosedWorkerID {
							// worker restarted
							log.Warnw("worker session changed (worker restarted?)", "initial", sessID, "current", curSes)
						}

						return
					}

					// session looks good
					if !enabled {
						sh.workersLk.Lock()
						worker.enabled = true
						sh.workersLk.Unlock()

						// we'll send window requests on the next loop
					}
				}

				select {
				case <-heartbeatTimer.C:
					continue
				case w := <-scheduledWindows:
					worker.wndLk.Lock()
					worker.activeWindows = append(worker.activeWindows, w)
					worker.wndLk.Unlock()
				case <-taskDone:
					log.Debugw("task done", "workerid", wid)
				case <-sh.closing:
					return
				case <-worker.closingMgr:
					return
				}

				break
			}

			// process assigned windows (non-blocking)
			sh.workersLk.RLock()
			worker.wndLk.Lock()

			windowsRequested -= sh.workerCompactWindows(worker, wid)
		assignLoop:
			// process windows in order
			for len(worker.activeWindows) > 0 {
				firstWindow := worker.activeWindows[0]

				// process tasks within a window, preferring tasks at lower indexes
				for len(firstWindow.todo) > 0 {
					tidx := -1

					worker.lk.Lock()
					for t, todo := range firstWindow.todo {
						needRes := ResourceTable[todo.taskType][sh.spt]
						if worker.preparing.canHandleRequest(needRes, wid, "startPreparing", worker.info.Resources) {
							tidx = t
							break
						}
					}
					worker.lk.Unlock()

					if tidx == -1 {
						break assignLoop
					}

					todo := firstWindow.todo[tidx]

					log.Debugf("assign worker sector %d", todo.sector.Number)
					err := sh.assignWorker(taskDone, wid, worker, todo)

					if err != nil {
						log.Error("assignWorker error: %+v", err)
						go todo.respond(xerrors.Errorf("assignWorker error: %w", err))
					}

					// Note: we're not freeing window.allocated resources here very much on purpose
					copy(firstWindow.todo[tidx:], firstWindow.todo[tidx+1:])
					firstWindow.todo[len(firstWindow.todo)-1] = nil
					firstWindow.todo = firstWindow.todo[:len(firstWindow.todo)-1]
				}

				copy(worker.activeWindows, worker.activeWindows[1:])
				worker.activeWindows[len(worker.activeWindows)-1] = nil
				worker.activeWindows = worker.activeWindows[:len(worker.activeWindows)-1]

				windowsRequested--
			}

			worker.wndLk.Unlock()
			sh.workersLk.RUnlock()
		}
	}()

	return nil
}

func (sh *scheduler) workerCompactWindows(worker *workerHandle, wid WorkerID) int {
	// move tasks from older windows to newer windows if older windows
	// still can fit them
	if len(worker.activeWindows) > 1 {
		for wi, window := range worker.activeWindows[1:] {
			lower := worker.activeWindows[wi]
			var moved []int

			for ti, todo := range window.todo {
				needRes := ResourceTable[todo.taskType][sh.spt]
				if !lower.allocated.canHandleRequest(needRes, wid, "compactWindows", worker.info.Resources) {
					continue
				}

				moved = append(moved, ti)
				lower.todo = append(lower.todo, todo)
				lower.allocated.add(worker.info.Resources, needRes)
				window.allocated.free(worker.info.Resources, needRes)
			}

			if len(moved) > 0 {
				newTodo := make([]*workerRequest, 0, len(window.todo)-len(moved))
				for i, t := range window.todo {
					if len(moved) > 0 && moved[0] == i {
						moved = moved[1:]
						continue
					}

					newTodo = append(newTodo, t)
				}
				window.todo = newTodo
			}
		}
	}

	var compacted int
	var newWindows []*schedWindow

	for _, window := range worker.activeWindows {
		if len(window.todo) == 0 {
			compacted++
			continue
		}

		newWindows = append(newWindows, window)
	}

	worker.activeWindows = newWindows

	return compacted
}

func (sh *scheduler) assignWorker(taskDone chan struct{}, wid WorkerID, w *workerHandle, req *workerRequest) error {
	needRes := ResourceTable[req.taskType][sh.spt]

	w.lk.Lock()
	w.preparing.add(w.info.Resources, needRes)
	w.lk.Unlock()

	go func() {
		err := req.prepare(req.ctx, sh.wt.worker(wid, w.w))
		sh.workersLk.Lock()

		if err != nil {
			w.lk.Lock()
			w.preparing.free(w.info.Resources, needRes)
			w.lk.Unlock()
			sh.workersLk.Unlock()

			select {
			case taskDone <- struct{}{}:
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

		err = w.active.withResources(wid, w.info.Resources, needRes, &sh.workersLk, func() error {
			w.lk.Lock()
			w.preparing.free(w.info.Resources, needRes)
			w.lk.Unlock()
			sh.workersLk.Unlock()
			defer sh.workersLk.Lock() // we MUST return locked from this function

			select {
			case taskDone <- struct{}{}:
			case <-sh.closing:
			}

			err = req.work(req.ctx, sh.wt.worker(wid, w.w))

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

func (sh *scheduler) workerCleanup(wid WorkerID, w *workerHandle) {
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

		newWindows := make([]*schedWindowRequest, 0, len(sh.openWindows))
		for _, window := range sh.openWindows {
			if window.worker != wid {
				newWindows = append(newWindows, window)
			}
		}
		sh.openWindows = newWindows

		log.Debugf("worker %d dropped", wid)
	}
}
