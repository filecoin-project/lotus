package sealer

import (
	"math"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func NewSpreadTasksAssigner(queued bool) Assigner {
	return &AssignerCommon{
		WindowSel: SpreadTasksWS(queued),
	}
}

type widTask struct {
	wid storiface.WorkerID
	tt  sealtasks.TaskType
}

func SpreadTasksWS(queued bool) func(sh *Scheduler, queueLen int, acceptableWindows [][]int, windows []SchedWindow) int {
	return func(sh *Scheduler, queueLen int, acceptableWindows [][]int, windows []SchedWindow) int {
		scheduled := 0
		rmQueue := make([]int, 0, queueLen)
		workerAssigned := map[widTask]int{}

		for sqi := 0; sqi < queueLen; sqi++ {
			task := (*sh.SchedQueue)[sqi]

			selectedWindow := -1
			var needRes storiface.Resources
			var info storiface.WorkerInfo
			var bestWid widTask
			bestAssigned := math.MaxInt // smaller = better

			for i, wnd := range acceptableWindows[task.IndexHeap] {
				wid := sh.OpenWindows[wnd].Worker
				w := sh.Workers[wid]

				res := w.Info.Resources.ResourceSpec(task.Sector.ProofType, task.TaskType)

				log.Debugf("SCHED try assign sqi:%d sector %d to window %d (awi:%d)", sqi, task.Sector.ID.Number, wnd, i)

				if !windows[wnd].Allocated.CanHandleRequest(task.SchedId, task.SealTask(), res, wid, "schedAssign", w.Info) {
					continue
				}

				wt := widTask{wid: wid, tt: task.TaskType}

				wu, found := workerAssigned[wt]
				if !found && queued {
					st := task.SealTask()
					wu = w.TaskCount(&st)
					workerAssigned[wt] = wu
				}
				if wu >= bestAssigned {
					continue
				}

				info = w.Info
				needRes = res
				bestWid = wt
				selectedWindow = wnd
				bestAssigned = wu
			}

			if selectedWindow < 0 {
				// all windows full
				continue
			}

			log.Debugw("SCHED ASSIGNED",
				"assigner", "spread-tasks",
				"spread-queued", queued,
				"sqi", sqi,
				"sector", task.Sector.ID.Number,
				"task", task.TaskType,
				"window", selectedWindow,
				"worker", bestWid,
				"assigned", bestAssigned)

			workerAssigned[bestWid]++
			windows[selectedWindow].Allocated.Add(task.SchedId, task.SealTask(), info.Resources, needRes)
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
}
