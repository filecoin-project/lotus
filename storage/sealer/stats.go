package sealer

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func (m *Manager) WorkerStats(ctx context.Context) map[uuid.UUID]storiface.WorkerStats {
	m.sched.workersLk.RLock()

	out := map[uuid.UUID]storiface.WorkerStats{}

	cb := func(ctx context.Context, id storiface.WorkerID, handle *WorkerHandle) {
		handle.lk.Lock()

		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		tt, err := handle.workerRpc.TaskTypes(ctx)
		var taskList []sealtasks.TaskType
		if err != nil {
			log.Warnw("getting worker task types in WorkerStats", "error", err)
		} else {
			for taskType := range tt {
				taskList = append(taskList, taskType)
			}
		}

		out[uuid.UUID(id)] = storiface.WorkerStats{
			Info:       handle.Info,
			Tasks:      taskList,
			Enabled:    handle.Enabled,
			MemUsedMin: handle.active.memUsedMin,
			MemUsedMax: handle.active.memUsedMax,
			GpuUsed:    handle.active.gpuUsed,
			CpuUse:     handle.active.cpuUse,

			TaskCounts: map[string]int{},
		}

		handle.active.taskCounters.ForEach(func(tt sealtasks.SealTaskType, count int) {
			out[uuid.UUID(id)].TaskCounts[tt.String()] = count
		})

		handle.lk.Unlock()
	}

	for id, handle := range m.sched.Workers {
		cb(ctx, id, handle)
	}

	m.sched.workersLk.RUnlock()

	//list post workers
	m.winningPoStSched.WorkerStats(ctx, cb)
	m.windowPoStSched.WorkerStats(ctx, cb)
	return out
}

func (m *Manager) WorkerJobs() map[uuid.UUID][]storiface.WorkerJob {
	out := map[uuid.UUID][]storiface.WorkerJob{}
	calls := map[storiface.CallID]struct{}{}

	running, preparing := m.sched.workTracker.Running()

	for _, t := range running {
		out[uuid.UUID(t.worker)] = append(out[uuid.UUID(t.worker)], t.job)
		calls[t.job.ID] = struct{}{}
	}
	for _, t := range preparing {
		out[uuid.UUID(t.worker)] = append(out[uuid.UUID(t.worker)], t.job)
		calls[t.job.ID] = struct{}{}
	}

	m.sched.workersLk.RLock()

	for id, handle := range m.sched.Workers {
		handle.wndLk.Lock()
		for wi, window := range handle.activeWindows {
			for _, request := range window.Todo {
				out[uuid.UUID(id)] = append(out[uuid.UUID(id)], storiface.WorkerJob{
					ID:      storiface.UndefCall,
					Sector:  request.Sector.ID,
					Task:    request.TaskType,
					RunWait: wi + 2,
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

		var ws WorkState
		if err := m.work.Get(work).Get(&ws); err != nil {
			log.Errorf("WorkerJobs: get work %s: %+v", work, err)
		}

		wait := storiface.RWRetWait
		if _, ok := m.results[work]; ok {
			wait = storiface.RWReturned
		}
		if ws.Status == wsDone {
			wait = storiface.RWRetDone
		}

		out[uuid.UUID{}] = append(out[uuid.UUID{}], storiface.WorkerJob{
			ID:       id,
			Sector:   id.Sector,
			Task:     work.Method,
			RunWait:  wait,
			Start:    time.Unix(ws.StartTime, 0),
			Hostname: ws.WorkerHostname,
		})
	}

	return out
}
