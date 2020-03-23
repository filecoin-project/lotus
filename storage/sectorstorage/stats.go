package sectorstorage

import "github.com/filecoin-project/lotus/api"

func (m *Manager) WorkerStats() map[uint64]api.WorkerStats {
	m.workersLk.Lock()
	defer m.workersLk.Unlock()

	out := map[uint64]api.WorkerStats{}

	for id, handle := range m.workers {
		out[uint64(id)] = api.WorkerStats{
			Info:       handle.info,
			MemUsedMin: handle.memUsedMin,
			MemUsedMax: handle.memUsedMax,
			GpuUsed:    handle.gpuUsed,
			CpuUse:     handle.cpuUse,
		}
	}

	return out
}
