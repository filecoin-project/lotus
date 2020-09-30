package sectorstorage

import (
	"time"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

func (m *Manager) WorkerStats() map[uint64]storiface.WorkerStats {
	m.sched.workersLk.RLock()
	defer m.sched.workersLk.RUnlock()

	out := map[uint64]storiface.WorkerStats{}

	for id, handle := range m.sched.workers {
		out[uint64(id)] = storiface.WorkerStats{
			Info:       handle.info,
			MemUsedMin: handle.active.memUsedMin,
			MemUsedMax: handle.active.memUsedMax,
			GpuUsed:    handle.active.gpuUsed,
			CpuUse:     handle.active.cpuUse,
		}
	}

	return out
}

func (m *Manager) WorkerJobs() map[int64][]storiface.WorkerJob {
	out := map[int64][]storiface.WorkerJob{}
	calls := map[storiface.CallID]struct{}{}

	for _, t := range m.sched.wt.Running() {
		out[int64(t.worker)] = append(out[int64(t.worker)], t.job)
		calls[t.job.ID] = struct{}{}
	}

	m.sched.workersLk.RLock()

	for id, handle := range m.sched.workers {
		handle.wndLk.Lock()
		for wi, window := range handle.activeWindows {
			for _, request := range window.todo {
				out[int64(id)] = append(out[int64(id)], storiface.WorkerJob{
					ID:      storiface.UndefCall,
					Sector:  request.sector,
					Task:    request.taskType,
					RunWait: wi + 1,
					Start:   request.start,
				})
			}
		}
		handle.wndLk.Unlock()
	}

	m.sched.workersLk.RUnlock()

	m.workLk.Lock()
	defer m.workLk.Unlock()

	for id, work := range m.callToWork {
		_, found := calls[id]
		if found {
			continue
		}

		out[-1] = append(out[-1], storiface.WorkerJob{
			ID:      id,
			Sector:  id.Sector,
			Task:    work.Method,
			RunWait: -1,
			Start:   time.Time{},
		})
	}

	return out
}
