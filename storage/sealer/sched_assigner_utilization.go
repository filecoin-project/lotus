package sealer

import (
	"math"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func NewLowestUtilizationAssigner() Assigner {
	return &AssignerCommon{
		WindowSel: LowestUtilizationWS,
	}
}

func LowestUtilizationWS(sh *Scheduler, queueLen int, acceptableWindows [][]int, windows []SchedWindow) int {
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

			res := w.Info.Resources.ResourceSpec(task.Sector.ProofType, task.TaskType)

			log.Debugf("SCHED try assign sqi:%d sector %d to window %d (awi:%d)", sqi, task.Sector.ID.Number, wnd, i)

			// TODO: allow bigger windows
			if !windows[wnd].Allocated.CanHandleRequest(task.SchedId, task.SealTask(), res, wid, "schedAssign", w.Info) {
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
			"assigner", "util",
			"sqi", sqi,
			"sector", task.Sector.ID.Number,
			"task", task.TaskType,
			"window", selectedWindow,
			"worker", bestWid,
			"utilization", bestUtilization)

		workerUtil[bestWid] += windows[selectedWindow].Allocated.Add(task.SchedId, task.SealTask(), info.Resources, needRes)
		windows[selectedWindow].Todo = append(windows[selectedWindow].Todo, task)

		rmQueue = append(rmQueue, sqi)
		scheduled++
	}

	if len(rmQueue) > 0 {
		for i := len(rmQueue) - 1; i >= 0; i-- {
			sh.SchedQueue.Remove(rmQueue[i])
		}
	}

	return scheduled
}
