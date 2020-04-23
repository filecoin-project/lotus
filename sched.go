package sectorstorage

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/storiface"
)

const mib = 1 << 20

type workerRequest struct {
	taskType sealtasks.TaskType
	accept   []WorkerID // ordered by preference

	ret    chan<- workerResponse
	cancel <-chan struct{}
}

type workerResponse struct {
	err error

	worker Worker
	done   func()
}

func (r *workerRequest) respond(resp workerResponse) {
	select {
	case r.ret <- resp:
	case <-r.cancel:
		log.Warnf("request got cancelled before we could respond")
		if resp.done != nil {
			resp.done()
		}
	}
}

type workerHandle struct {
	w Worker

	info storiface.WorkerInfo

	memUsedMin uint64
	memUsedMax uint64
	gpuUsed    bool
	cpuUse     int // -1 - multicore thing; 0 - free; 1+ - singlecore things
}

func (m *Manager) runSched() {
	for {
		select {
		case w := <-m.newWorkers:
			m.schedNewWorker(w)
		case req := <-m.schedule:
			resp, err := m.maybeSchedRequest(req)
			if err != nil {
				req.respond(workerResponse{err: err})
				continue
			}

			if resp != nil {
				req.respond(*resp)
				continue
			}

			m.schedQueue.PushBack(req)
		case wid := <-m.workerFree:
			m.onWorkerFreed(wid)
		case <-m.closing:
			m.schedClose()
			return
		}
	}
}

func (m *Manager) onWorkerFreed(wid WorkerID) {
	for e := m.schedQueue.Front(); e != nil; e = e.Next() {
		req := e.Value.(*workerRequest)
		var ok bool
		for _, id := range req.accept {
			if id == wid {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}

		resp, err := m.maybeSchedRequest(req)
		if err != nil {
			req.respond(workerResponse{err: err})
			continue
		}

		if resp != nil {
			req.respond(*resp)

			pe := e.Prev()
			m.schedQueue.Remove(e)
			if pe == nil {
				pe = m.schedQueue.Front()
			}
			if pe == nil {
				break
			}
			e = pe
			continue
		}
	}
}

func (m *Manager) maybeSchedRequest(req *workerRequest) (*workerResponse, error) {
	m.workersLk.Lock()
	defer m.workersLk.Unlock()

	tried := 0

	for i := len(req.accept) - 1; i >= 0; i-- {
		id := req.accept[i]
		w, ok := m.workers[id]
		if !ok {
			log.Warnf("requested worker %d is not in scheduler", id)
		}
		tried++

		canDo, err := m.canHandleRequest(id, w, req)
		if err != nil {
			return nil, err
		}

		if !canDo {
			continue
		}

		return m.makeResponse(id, w, req), nil
	}

	if tried == 0 {
		return nil, xerrors.New("maybeSchedRequest didn't find any good workers")
	}

	return nil, nil // put in waiting queue
}

func (m *Manager) makeResponse(wid WorkerID, w *workerHandle, req *workerRequest) *workerResponse {
	needRes := ResourceTable[req.taskType][m.scfg.SealProofType]

	w.gpuUsed = needRes.CanGPU
	if needRes.MultiThread {
		w.cpuUse = -1
	} else {
		if w.cpuUse != -1 {
			w.cpuUse++
		} else {
			log.Warnf("sched: makeResponse for worker %d: worker cpu is in multicore use, but a single core task was scheduled", wid)
		}
	}

	w.memUsedMin += needRes.MinMemory
	w.memUsedMax += needRes.MaxMemory

	return &workerResponse{
		err:    nil,
		worker: w.w,
		done: func() {
			m.workersLk.Lock()

			if needRes.CanGPU {
				w.gpuUsed = false
			}

			if needRes.MultiThread {
				w.cpuUse = 0
			} else if w.cpuUse != -1 {
				w.cpuUse--
			}

			w.memUsedMin -= needRes.MinMemory
			w.memUsedMax -= needRes.MaxMemory

			m.workersLk.Unlock()

			select {
			case m.workerFree <- wid:
			case <-m.closing:
			}
		},
	}
}

func (m *Manager) canHandleRequest(wid WorkerID, w *workerHandle, req *workerRequest) (bool, error) {
	needRes, ok := ResourceTable[req.taskType][m.scfg.SealProofType]
	if !ok {
		return false, xerrors.Errorf("canHandleRequest: missing ResourceTable entry for %s/%d", req.taskType, m.scfg.SealProofType)
	}

	res := w.info.Resources

	// TODO: dedupe needRes.BaseMinMemory per task type (don't add if that task is already running)
	minNeedMem := res.MemReserved + w.memUsedMin + needRes.MinMemory + needRes.BaseMinMemory
	if minNeedMem > res.MemPhysical {
		log.Debugf("sched: not scheduling on worker %d; not enough physical memory - need: %dM, have %dM", wid, minNeedMem/mib, res.MemPhysical/mib)
		return false, nil
	}

	maxNeedMem := res.MemReserved + w.memUsedMax + needRes.MaxMemory + needRes.BaseMinMemory
	if m.scfg.SealProofType == abi.RegisteredProof_StackedDRG32GiBSeal {
		maxNeedMem += MaxCachingOverhead
	}
	if maxNeedMem > res.MemSwap+res.MemPhysical {
		log.Debugf("sched: not scheduling on worker %d; not enough virtual memory - need: %dM, have %dM", wid, maxNeedMem/mib, (res.MemSwap+res.MemPhysical)/mib)
		return false, nil
	}

	if needRes.MultiThread {
		if w.cpuUse != 0 {
			log.Debugf("sched: not scheduling on worker %d; multicore process needs free CPU", wid)
			return false, nil
		}
	} else {
		if w.cpuUse == -1 {
			log.Debugf("sched: not scheduling on worker %d; CPU in use by a multicore process", wid)
			return false, nil
		}
	}

	if len(res.GPUs) > 0 && needRes.CanGPU {
		if w.gpuUsed {
			log.Debugf("sched: not scheduling on worker %d; GPU in use", wid)
			return false, nil
		}
	}

	return true, nil
}

func (m *Manager) schedNewWorker(w *workerHandle) {
	m.workersLk.Lock()
	defer m.workersLk.Unlock()

	id := m.nextWorker
	m.workers[id] = w
	m.nextWorker++
}

func (m *Manager) schedClose() {
	m.workersLk.Lock()
	defer m.workersLk.Unlock()

	for i, w := range m.workers {
		if err := w.w.Close(); err != nil {
			log.Errorf("closing worker %d: %+v", i, err)
		}
	}
}
