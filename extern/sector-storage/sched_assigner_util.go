package sectorstorage

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"sync"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

// AssignerUtil is a task assigner assigning tasks to workers with lowest utilization
type AssignerUtil struct{}

var _ Assigner = &AssignerUtil{}

func (a *AssignerUtil) TrySched(sh *Scheduler) {
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

	windowsLen := len(sh.OpenWindows)
	queueLen := sh.SchedQueue.Len()

	log.Debugf("SCHED %d queued; %d open windows", queueLen, windowsLen)

	if windowsLen == 0 || queueLen == 0 {
		// nothing to schedule on
		return
	}

	windows := make([]SchedWindow, windowsLen)
	acceptableWindows := make([][]int, queueLen) // QueueIndex -> []OpenWindowIndex

	// Step 1
	throttle := make(chan struct{}, windowsLen)

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
			for wnd, windowRequest := range sh.OpenWindows {
				worker, ok := sh.Workers[windowRequest.Worker]
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
				if !windows[wnd].Allocated.CanHandleRequest(needRes, windowRequest.Worker, "schedAcceptable", worker.Info) {
					continue
				}

				rpcCtx, cancel := context.WithTimeout(task.Ctx, SelectorTimeout)
				ok, err := task.Sel.Ok(rpcCtx, task.TaskType, task.Sector.ProofType, worker)
				cancel()
				if err != nil {
					log.Errorf("trySched(1) req.Sel.Ok error: %+v", err)
					continue
				}

				if !ok {
					continue
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

				wi := sh.Workers[wii]
				wj := sh.Workers[wji]

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

	log.Debugf("SCHED windows: %+v", windows)
	log.Debugf("SCHED Acceptable win: %+v", acceptableWindows)

	// Step 2
	scheduled := 0
	rmQueue := make([]int, 0, queueLen)
	workerUtil := map[storiface.WorkerID]float64{}

	for sqi := 0; sqi < queueLen; sqi++ {
		task := (*sh.SchedQueue)[sqi]

		selectedWindow := -1
		var needRes storiface.Resources
		var info storiface.WorkerInfo
		var bestWid storiface.WorkerID
		bestUtilization := math.MaxFloat64 // smaller = better

		for i, wnd := range acceptableWindows[task.IndexHeap] {
			wid := sh.OpenWindows[wnd].Worker
			w := sh.Workers[wid]

			res := info.Resources.ResourceSpec(task.Sector.ProofType, task.TaskType)

			log.Debugf("SCHED try assign sqi:%d sector %d to window %d (awi:%d)", sqi, task.Sector.ID.Number, wnd, i)

			// TODO: allow bigger windows
			if !windows[wnd].Allocated.CanHandleRequest(needRes, wid, "schedAssign", info) {
				continue
			}

			wu, found := workerUtil[wid]
			if !found {
				wu = w.Utilization()
				workerUtil[wid] = wu
			}
			if wu >= bestUtilization {
				// acceptable worker list is initially sorted by utilization, and the initially-best workers
				// will be assigned tasks first. This means that if we find a worker which isn't better, it
				// probably means that the other workers aren't better either.
				//
				// utilization
				// ^
				// |       /
				// | \    /
				// |  \  /
				// |   *
				// #--------> acceptableWindow index
				//
				// * -> we're here
				break
			}

			info = w.Info
			needRes = res
			bestWid = wid
			selectedWindow = wnd
			bestUtilization = wu
		}

		if selectedWindow < 0 {
			// all windows full
			continue
		}

		log.Debugw("SCHED ASSIGNED",
			"sqi", sqi,
			"sector", task.Sector.ID.Number,
			"task", task.TaskType,
			"window", selectedWindow,
			"worker", bestWid,
			"utilization", bestUtilization)

		workerUtil[bestWid] += windows[selectedWindow].Allocated.Add(info.Resources, needRes)
		windows[selectedWindow].Todo = append(windows[selectedWindow].Todo, task)

		rmQueue = append(rmQueue, sqi)
		scheduled++
	}

	if len(rmQueue) > 0 {
		for i := len(rmQueue) - 1; i >= 0; i-- {
			sh.SchedQueue.Remove(rmQueue[i])
		}
	}

	// Step 3

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
