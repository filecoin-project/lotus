package sealer

import (
	"sync"

	"github.com/google/uuid"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type ActiveResources struct {
	memUsedMin uint64
	memUsedMax uint64
	gpuUsed    float64
	cpuUse     uint64

	taskCounters *taskCounter

	cond    *sync.Cond
	waiting int
}

type taskCounter struct {
	taskCounters map[sealtasks.SealTaskType]map[uuid.UUID]int

	// this lock is technically redundant, as ActiveResources is always accessed
	// with the worker lock, but let's not panic if we ever change that
	lk sync.Mutex
}

func newTaskCounter() *taskCounter {
	return &taskCounter{
		taskCounters: make(map[sealtasks.SealTaskType]map[uuid.UUID]int),
	}
}

func (tc *taskCounter) Add(tt sealtasks.SealTaskType, schedID uuid.UUID) {
	tc.lk.Lock()
	defer tc.lk.Unlock()
	tc.getUnlocked(tt)[schedID]++
}

func (tc *taskCounter) Free(tt sealtasks.SealTaskType, schedID uuid.UUID) {
	tc.lk.Lock()
	defer tc.lk.Unlock()
	m := tc.getUnlocked(tt)
	if m[schedID] <= 1 {
		delete(m, schedID)
	} else {
		m[schedID]--
	}
}

func (tc *taskCounter) getUnlocked(tt sealtasks.SealTaskType) map[uuid.UUID]int {
	if tc.taskCounters[tt] == nil {
		tc.taskCounters[tt] = make(map[uuid.UUID]int)
	}

	return tc.taskCounters[tt]
}

func (tc *taskCounter) Get(tt sealtasks.SealTaskType) map[uuid.UUID]int {
	tc.lk.Lock()
	defer tc.lk.Unlock()

	return tc.getUnlocked(tt)
}

func (tc *taskCounter) Sum() int {
	tc.lk.Lock()
	defer tc.lk.Unlock()
	sum := 0
	for _, v := range tc.taskCounters {
		sum += len(v)
	}
	return sum
}

func (tc *taskCounter) ForEach(cb func(tt sealtasks.SealTaskType, count int)) {
	tc.lk.Lock()
	defer tc.lk.Unlock()
	for tt, v := range tc.taskCounters {
		cb(tt, len(v))
	}
}

func NewActiveResources(tc *taskCounter) *ActiveResources {
	return &ActiveResources{
		taskCounters: tc,
	}
}

func (a *ActiveResources) withResources(schedID uuid.UUID, id storiface.WorkerID, wr storiface.WorkerInfo, tt sealtasks.SealTaskType, r storiface.Resources, locker sync.Locker, cb func() error) error {
	for !a.CanHandleRequest(schedID, tt, r, id, "withResources", wr) {
		if a.cond == nil {
			a.cond = sync.NewCond(locker)
		}
		a.waiting++
		a.cond.Wait()
		a.waiting--
	}

	a.Add(schedID, tt, wr.Resources, r)

	err := cb()

	a.Free(schedID, tt, wr.Resources, r)

	return err
}

// must be called with the same lock as the one passed to withResources
func (a *ActiveResources) hasWorkWaiting() bool {
	return a.waiting > 0
}

// Add task resources to ActiveResources and return utilization difference
func (a *ActiveResources) Add(schedID uuid.UUID, tt sealtasks.SealTaskType, wr storiface.WorkerResources, r storiface.Resources) float64 {
	startUtil := a.utilization(wr)

	if r.GPUUtilization > 0 {
		a.gpuUsed += r.GPUUtilization
	}
	a.cpuUse += r.Threads(wr.CPUs, len(wr.GPUs))
	a.memUsedMin += r.MinMemory
	a.memUsedMax += r.MaxMemory
	a.taskCounters.Add(tt, schedID)

	return a.utilization(wr) - startUtil
}

func (a *ActiveResources) Free(schedID uuid.UUID, tt sealtasks.SealTaskType, wr storiface.WorkerResources, r storiface.Resources) {
	if r.GPUUtilization > 0 {
		a.gpuUsed -= r.GPUUtilization
	}
	a.cpuUse -= r.Threads(wr.CPUs, len(wr.GPUs))
	a.memUsedMin -= r.MinMemory
	a.memUsedMax -= r.MaxMemory
	a.taskCounters.Free(tt, schedID)

	if a.cond != nil {
		a.cond.Broadcast()
	}
}

// CanHandleRequest evaluates if the worker has enough available resources to
// handle the request.
func (a *ActiveResources) CanHandleRequest(schedID uuid.UUID, tt sealtasks.SealTaskType, needRes storiface.Resources, wid storiface.WorkerID, caller string, info storiface.WorkerInfo) bool {
	if needRes.MaxConcurrent > 0 {
		tasks := a.taskCounters.Get(tt)
		if len(tasks) >= needRes.MaxConcurrent && (schedID == uuid.UUID{} || tasks[schedID] == 0) {
			log.Debugf("sched: not scheduling on worker %s for %s; at task limit tt=%s, curcount=%d", wid, caller, tt, a.taskCounters.Get(tt))
			return false
		}
	}

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

// utilization returns a number in 0..1 range indicating fraction of used resources
func (a *ActiveResources) utilization(wr storiface.WorkerResources) float64 { // todo task type
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

	if len(wr.GPUs) > 0 {
		gpuMax := a.gpuUsed / float64(len(wr.GPUs))
		if gpuMax > max {
			max = gpuMax
		}
	}

	return max
}

func (a *ActiveResources) taskCount(tt *sealtasks.SealTaskType) int {
	// nil means all tasks
	if tt == nil {
		return a.taskCounters.Sum()
	}

	return len(a.taskCounters.Get(*tt))
}

func (wh *WorkerHandle) Utilization() float64 {
	wh.lk.Lock()
	u := wh.active.utilization(wh.Info.Resources)
	u += wh.preparing.utilization(wh.Info.Resources)
	wh.lk.Unlock()
	wh.wndLk.Lock()
	for _, window := range wh.activeWindows {
		u += window.Allocated.utilization(wh.Info.Resources)
	}
	wh.wndLk.Unlock()

	return u
}

func (wh *WorkerHandle) TaskCounts() int {
	wh.lk.Lock()
	u := wh.active.taskCount(nil)
	u += wh.preparing.taskCount(nil)
	wh.lk.Unlock()
	wh.wndLk.Lock()
	for _, window := range wh.activeWindows {
		u += window.Allocated.taskCount(nil)
	}
	wh.wndLk.Unlock()

	return u
}

func (wh *WorkerHandle) TaskCount(tt *sealtasks.SealTaskType) int {
	wh.lk.Lock()
	u := wh.active.taskCount(tt)
	u += wh.preparing.taskCount(tt)
	wh.lk.Unlock()
	wh.wndLk.Lock()
	for _, window := range wh.activeWindows {
		u += window.Allocated.taskCount(tt)
	}
	wh.wndLk.Unlock()

	return u
}
