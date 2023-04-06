package sealer

import (
	"math/rand"

	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func NewRandomAssigner() Assigner {
	return &AssignerCommon{
		WindowSel: RandomWS,
	}
}

func RandomWS(sh *Scheduler, queueLen int, acceptableWindows [][]int, windows []SchedWindow) int {
	scheduled := 0
	rmQueue := make([]int, 0, queueLen)

	for sqi := 0; sqi < queueLen; sqi++ {
		task := (*sh.SchedQueue)[sqi]

		//bestAssigned := math.MaxInt // smaller = better

		type choice struct {
			selectedWindow int
			needRes        storiface.Resources
			info           storiface.WorkerInfo
			bestWid        storiface.WorkerID
		}
		choices := make([]choice, 0, len(acceptableWindows[task.IndexHeap]))

		for i, wnd := range acceptableWindows[task.IndexHeap] {
			wid := sh.OpenWindows[wnd].Worker
			w := sh.Workers[wid]

			res := w.Info.Resources.ResourceSpec(task.Sector.ProofType, task.TaskType)

			log.Debugf("SCHED try assign sqi:%d sector %d to window %d (awi:%d)", sqi, task.Sector.ID.Number, wnd, i)

			if !windows[wnd].Allocated.CanHandleRequest(task.SchedId, task.SealTask(), res, wid, "schedAssign", w.Info) {
				continue
			}

			choices = append(choices, choice{
				selectedWindow: wnd,
				needRes:        res,
				info:           w.Info,
				bestWid:        wid,
			})

		}

		if len(choices) == 0 {
			// all windows full
			continue
		}

		// chose randomly
		randIndex := rand.Intn(len(choices))
		selectedWindow := choices[randIndex].selectedWindow
		needRes := choices[randIndex].needRes
		info := choices[randIndex].info
		bestWid := choices[randIndex].bestWid

		log.Debugw("SCHED ASSIGNED",
			"assigner", "darts",
			"sqi", sqi,
			"sector", task.Sector.ID.Number,
			"task", task.TaskType,
			"window", selectedWindow,
			"worker", bestWid,
			"choices", len(choices))

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
