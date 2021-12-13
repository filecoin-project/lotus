package sectorstorage

import (
	"sync"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

func (a *activeResources) withResources(id storiface.WorkerID, wr storiface.WorkerInfo, r storiface.Resources, locker sync.Locker, cb func() error) error {
	for !a.canHandleRequest(r, id, "withResources", wr) {
		if a.cond == nil {
			a.cond = sync.NewCond(locker)
		}
		a.waiting++
		a.cond.Wait()
		a.waiting--
	}

	a.add(wr.Resources, r)

	err := cb()

	a.free(wr.Resources, r)

	return err
}

// must be called with the same lock as the one passed to withResources
func (a *activeResources) hasWorkWaiting() bool {
	return a.waiting > 0
}

func (a *activeResources) add(wr storiface.WorkerResources, r storiface.Resources) {
	if r.GPUUtilization > 0 {
		a.gpuUsed += r.GPUUtilization
	}
	a.cpuUse += r.Threads(wr.CPUs, len(wr.GPUs))
	a.memUsedMin += r.MinMemory
	a.memUsedMax += r.MaxMemory
}

func (a *activeResources) free(wr storiface.WorkerResources, r storiface.Resources) {
	if r.GPUUtilization > 0 {
		a.gpuUsed -= r.GPUUtilization
	}
	a.cpuUse -= r.Threads(wr.CPUs, len(wr.GPUs))
	a.memUsedMin -= r.MinMemory
	a.memUsedMax -= r.MaxMemory

	if a.cond != nil {
		a.cond.Broadcast()
	}
}

// canHandleRequest evaluates if the worker has enough available resources to
// handle the request.
func (a *activeResources) canHandleRequest(needRes storiface.Resources, wid storiface.WorkerID, caller string, info storiface.WorkerInfo) bool {
	if info.IgnoreResources {
		// shortcircuit; if this worker is ignoring resources, it can always handle the request.
		return true
	}

	res := info.Resources

	// TODO: dedupe needRes.BaseMinMemory per task type (don't add if that task is already running)
	memNeeded := needRes.MinMemory + needRes.BaseMinMemory
	memUsed := a.memUsedMin
	// assume that MemUsed can be swapped, so only check it in the vmem Check
	memAvail := res.MemPhysical - memUsed
	if memNeeded > memAvail {
		log.Debugf("sched: not scheduling on worker %s for %s; not enough physical memory - need: %dM, have %dM available", wid, caller, memNeeded/mib, memAvail/mib)
		return false
	}

	vmemNeeded := needRes.MaxMemory + needRes.BaseMinMemory
	vmemUsed := a.memUsedMax
	workerMemoryReserved := res.MemUsed + res.MemSwapUsed // memory used outside lotus-worker (used by the OS, etc.)

	if vmemUsed < workerMemoryReserved {
		vmemUsed = workerMemoryReserved
	}
	vmemAvail := (res.MemPhysical + res.MemSwap) - vmemUsed

	if vmemNeeded > vmemAvail {
		log.Debugf("sched: not scheduling on worker %s for %s; not enough virtual memory - need: %dM, have %dM available", wid, caller, vmemNeeded/mib, vmemAvail/mib)
		return false
	}

	if a.cpuUse+needRes.Threads(res.CPUs, len(res.GPUs)) > res.CPUs {
		log.Debugf("sched: not scheduling on worker %s for %s; not enough threads, need %d, %d in use, target %d", wid, caller, needRes.Threads(res.CPUs, len(res.GPUs)), a.cpuUse, res.CPUs)
		return false
	}

	if len(res.GPUs) > 0 && needRes.GPUUtilization > 0 {
		if a.gpuUsed+needRes.GPUUtilization > float64(len(res.GPUs)) {
			log.Debugf("sched: not scheduling on worker %s for %s; GPU(s) in use", wid, caller)
			return false
		}
	}

	return true
}

func (a *activeResources) utilization(wr storiface.WorkerResources) float64 {
	var max float64

	cpu := float64(a.cpuUse) / float64(wr.CPUs)
	max = cpu

	memUsed := a.memUsedMin
	if memUsed < wr.MemUsed {
		memUsed = wr.MemUsed
	}
	memMin := float64(memUsed) / float64(wr.MemPhysical)
	if memMin > max {
		max = memMin
	}

	vmemUsed := a.memUsedMax
	if a.memUsedMax < wr.MemUsed+wr.MemSwapUsed {
		vmemUsed = wr.MemUsed + wr.MemSwapUsed
	}
	memMax := float64(vmemUsed) / float64(wr.MemPhysical+wr.MemSwap)

	if memMax > max {
		max = memMax
	}

	return max
}

func (wh *workerHandle) utilization() float64 {
	wh.lk.Lock()
	u := wh.active.utilization(wh.info.Resources)
	u += wh.preparing.utilization(wh.info.Resources)
	wh.lk.Unlock()
	wh.wndLk.Lock()
	for _, window := range wh.activeWindows {
		u += window.allocated.utilization(wh.info.Resources)
	}
	wh.wndLk.Unlock()

	return u
}
