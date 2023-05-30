package sealer

import (
	"context"
	"math/rand"
	"sort"
	"sync"

	"go.opencensus.io/stats"

	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type WindowSelector func(sh *Scheduler, queueLen int, acceptableWindows [][]int, windows []SchedWindow) int

// AssignerCommon is a task assigner with customizable parts
type AssignerCommon struct {
	WindowSel WindowSelector
}

var _ Assigner = &AssignerCommon{}

func (a *AssignerCommon) TrySched(sh *Scheduler) {
	/*
		This assigns tasks to workers based on:
		- Task priority (achieved by handling sh.SchedQueue in order, since it's already sorted by priority)
		- Worker resource availability
		- Task-specified worker preference (acceptableWindows array below sorted by this preference)
		- Window request age

		1. For each task in the SchedQueue find windows which can handle them
		1.1. Create list of windows capable of handling a task
		1.2. Sort windows according to task selector preferences
		2. Going through SchedQueue again, assign task to first acceptable window
		   with resources available
		3. Submit windows with scheduled tasks to workers

	*/

	cachedWorkers := &schedWorkerCache{
		Workers: sh.Workers,
		cached:  map[storiface.WorkerID]*cachedSchedWorker{},
	}

	windowsLen := len(sh.OpenWindows)
	queueLen := sh.SchedQueue.Len()

	stats.Record(sh.mctx, metrics.SchedCycleOpenWindows.M(int64(windowsLen)))
	stats.Record(sh.mctx, metrics.SchedCycleQueueSize.M(int64(queueLen)))

	log.Debugf("SCHED %d queued; %d open windows", queueLen, windowsLen)

	if windowsLen == 0 || queueLen == 0 {
		// nothing to schedule on
		return
	}

	windows := make([]SchedWindow, windowsLen)
	for i := range windows {
		windows[i].Allocated = *NewActiveResources(newTaskCounter())
	}
	acceptableWindows := make([][]int, queueLen) // QueueIndex -> []OpenWindowIndex

	// Step 1
	throttle := make(chan struct{}, windowsLen)

	partDone := metrics.Timer(sh.mctx, metrics.SchedAssignerCandidatesDuration)
	defer func() {
		// call latest value of partDone in case we error out somewhere
		partDone()
	}()

	var wg sync.WaitGroup
	wg.Add(queueLen)
	for i := 0; i < queueLen; i++ {
		throttle <- struct{}{}

		go func(sqi int) {
			defer wg.Done()
			defer func() {
				<-throttle
			}()

			task := (*sh.SchedQueue)[sqi]
			task.IndexHeap = sqi

			var havePreferred bool

			for wnd, windowRequest := range sh.OpenWindows {
				worker, ok := cachedWorkers.Get(windowRequest.Worker)
				if !ok {
					log.Errorf("worker referenced by windowRequest not found (worker: %s)", windowRequest.Worker)
					// TODO: How to move forward here?
					continue
				}

				if !worker.Enabled {
					log.Debugw("skipping disabled worker", "worker", windowRequest.Worker)
					continue
				}

				needRes := worker.Info.Resources.ResourceSpec(task.Sector.ProofType, task.TaskType)

				// TODO: allow bigger windows
				if !windows[wnd].Allocated.CanHandleRequest(task.SchedId, task.SealTask(), needRes, windowRequest.Worker, "schedAcceptable", worker.Info) {
					continue
				}

				rpcCtx, cancel := context.WithTimeout(task.Ctx, SelectorTimeout)
				ok, preferred, err := task.Sel.Ok(rpcCtx, task.TaskType, task.Sector.ProofType, worker)
				cancel()
				if err != nil {
					log.Errorf("trySched(1) req.Sel.Ok error: %+v", err)
					continue
				}

				if !ok {
					continue
				}

				if havePreferred && !preferred {
					// we have a way better worker for this task
					continue
				}

				if preferred && !havePreferred {
					// all workers we considered previously are much worse choice
					acceptableWindows[sqi] = acceptableWindows[sqi][:0]
					havePreferred = true
				}

				acceptableWindows[sqi] = append(acceptableWindows[sqi], wnd)
			}

			if len(acceptableWindows[sqi]) == 0 {
				return
			}

			// Pick best worker (shuffle in case some workers are equally as good)
			rand.Shuffle(len(acceptableWindows[sqi]), func(i, j int) {
				acceptableWindows[sqi][i], acceptableWindows[sqi][j] = acceptableWindows[sqi][j], acceptableWindows[sqi][i] // nolint:scopelint
			})
			sort.SliceStable(acceptableWindows[sqi], func(i, j int) bool {
				wii := sh.OpenWindows[acceptableWindows[sqi][i]].Worker // nolint:scopelint
				wji := sh.OpenWindows[acceptableWindows[sqi][j]].Worker // nolint:scopelint

				if wii == wji {
					// for the same worker prefer older windows
					return acceptableWindows[sqi][i] < acceptableWindows[sqi][j] // nolint:scopelint
				}

				wi, _ := cachedWorkers.Get(wii)
				wj, _ := cachedWorkers.Get(wji)

				rpcCtx, cancel := context.WithTimeout(task.Ctx, SelectorTimeout)
				defer cancel()

				r, err := task.Sel.Cmp(rpcCtx, task.TaskType, wi, wj)
				if err != nil {
					log.Errorf("selecting best worker: %s", err)
				}
				return r
			})
		}(i)
	}

	wg.Wait()

	log.Debugf("SCHED Acceptable win: %+v", acceptableWindows)

	// Step 2
	partDone()
	partDone = metrics.Timer(sh.mctx, metrics.SchedAssignerWindowSelectionDuration)

	scheduled := a.WindowSel(sh, queueLen, acceptableWindows, windows)

	// Step 3
	partDone()
	partDone = metrics.Timer(sh.mctx, metrics.SchedAssignerSubmitDuration)

	if scheduled == 0 {
		return
	}

	scheduledWindows := map[int]struct{}{}
	for wnd, window := range windows {
		if len(window.Todo) == 0 {
			// Nothing scheduled here, keep the window open
			continue
		}

		scheduledWindows[wnd] = struct{}{}

		window := window // copy
		select {
		case sh.OpenWindows[wnd].Done <- &window:
		default:
			log.Error("expected sh.OpenWindows[wnd].Done to be buffered")
		}
	}

	// Rewrite sh.OpenWindows array, removing scheduled windows
	newOpenWindows := make([]*SchedWindowRequest, 0, windowsLen-len(scheduledWindows))
	for wnd, window := range sh.OpenWindows {
		if _, scheduled := scheduledWindows[wnd]; scheduled {
			// keep unscheduled windows open
			continue
		}

		newOpenWindows = append(newOpenWindows, window)
	}

	sh.OpenWindows = newOpenWindows
}
