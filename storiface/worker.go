package storiface

type WorkerInfo struct {
	Hostname string

	Resources WorkerResources
}

type WorkerResources struct {
	MemPhysical uint64
	MemSwap     uint64

	MemReserved uint64 // Used by system / other processes

	GPUs []string
}

type WorkerStats struct {
	Info WorkerInfo

	MemUsedMin uint64
	MemUsedMax uint64
	GpuUsed    bool
	CpuUse     int
}
