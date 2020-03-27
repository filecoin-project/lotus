package sectorstorage

type WorkerStats struct {
	Info WorkerInfo

	MemUsedMin uint64
	MemUsedMax uint64
	GpuUsed    bool
	CpuUse     int
}

func (m *Manager) WorkerStats() map[uint64]WorkerStats {
	m.workersLk.Lock()
	defer m.workersLk.Unlock()

	out := map[uint64]WorkerStats{}

	for id, handle := range m.workers {
		out[uint64(id)] = WorkerStats{
			Info:       handle.info,
			MemUsedMin: handle.memUsedMin,
			MemUsedMax: handle.memUsedMax,
			GpuUsed:    handle.gpuUsed,
			CpuUse:     handle.cpuUse,
		}
	}

	return out
}
