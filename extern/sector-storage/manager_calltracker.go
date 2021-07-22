package sectorstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type WorkID struct {
	Method sealtasks.TaskType
	Params string // json [...params]
}

func (w WorkID) String() string {
	return fmt.Sprintf("%s(%s)", w.Method, w.Params)
}

var _ fmt.Stringer = &WorkID{}

type WorkStatus string

const (
	wsStarted WorkStatus = "started" // task started, not scheduled/running on a worker yet
	wsRunning WorkStatus = "running" // task running on a worker, waiting for worker return
	wsDone    WorkStatus = "done"    // task returned from the worker, results available
)

type WorkState struct {
	ID WorkID

	Status WorkStatus

	WorkerCall storiface.CallID // Set when entering wsRunning
	WorkError  string           // Status = wsDone, set when failed to start work

	WorkerHostname string // hostname of last worker handling this job
	StartTime      int64  // unix seconds
}

func newWorkID(method sealtasks.TaskType, params ...interface{}) (WorkID, error) {
	pb, err := json.Marshal(params)
	if err != nil {
		return WorkID{}, xerrors.Errorf("marshaling work params: %w", err)
	}

	if len(pb) > 256 {
		s := sha256.Sum256(pb)
		pb = []byte(hex.EncodeToString(s[:]))
	}

	return WorkID{
		Method: method,
		Params: string(pb),
	}, nil
}

func (m *Manager) setupWorkTracker() {
	m.workLk.Lock()
	defer m.workLk.Unlock()

	var ids []WorkState
	if err := m.work.List(&ids); err != nil {
		log.Error("getting work IDs") // quite bad
		return
	}

	for _, st := range ids {
		wid := st.ID

		if os.Getenv("LOTUS_MINER_ABORT_UNFINISHED_WORK") == "1" {
			st.Status = wsDone
		}

		switch st.Status {
		case wsStarted:
			log.Warnf("dropping non-running work %s", wid)

			if err := m.work.Get(wid).End(); err != nil {
				log.Errorf("cleannig up work state for %s", wid)
			}
		case wsDone:
			// can happen after restart, abandoning work, and another restart
			log.Warnf("dropping done work, no result, wid %s", wid)

			if err := m.work.Get(wid).End(); err != nil {
				log.Errorf("cleannig up work state for %s", wid)
			}
		case wsRunning:
			m.callToWork[st.WorkerCall] = wid
		}
	}
}

// returns wait=true when the task is already tracked/running
func (m *Manager) getWork(ctx context.Context, method sealtasks.TaskType, params ...interface{}) (wid WorkID, wait bool, cancel func(), err error) {
	wid, err = newWorkID(method, params)
	if err != nil {
		return WorkID{}, false, nil, xerrors.Errorf("creating WorkID: %w", err)
	}

	m.workLk.Lock()
	defer m.workLk.Unlock()

	have, err := m.work.Has(wid)
	if err != nil {
		return WorkID{}, false, nil, xerrors.Errorf("failed to check if the task is already tracked: %w", err)
	}

	if !have {
		err := m.work.Begin(wid, &WorkState{
			ID:     wid,
			Status: wsStarted,
		})
		if err != nil {
			return WorkID{}, false, nil, xerrors.Errorf("failed to track task start: %w", err)
		}

		return wid, false, func() {
			m.workLk.Lock()
			defer m.workLk.Unlock()

			have, err := m.work.Has(wid)
			if err != nil {
				log.Errorf("cancel: work has error: %+v", err)
				return
			}

			if !have {
				return // expected / happy path
			}

			var ws WorkState
			if err := m.work.Get(wid).Get(&ws); err != nil {
				log.Errorf("cancel: get work %s: %+v", wid, err)
				return
			}

			switch ws.Status {
			case wsStarted:
				log.Warnf("canceling started (not running) work %s", wid)

				if err := m.work.Get(wid).End(); err != nil {
					log.Errorf("cancel: failed to cancel started work %s: %+v", wid, err)
					return
				}
			case wsDone:
				// TODO: still remove?
				log.Warnf("cancel called on work %s in 'done' state", wid)
			case wsRunning:
				log.Warnf("cancel called on work %s in 'running' state (manager shutting down?)", wid)
			}

		}, nil
	}

	// already started

	return wid, true, func() {
		// TODO
	}, nil
}

func (m *Manager) startWork(ctx context.Context, w Worker, wk WorkID) func(callID storiface.CallID, err error) error {
	return func(callID storiface.CallID, err error) error {
		var hostname string
		info, ierr := w.Info(ctx)
		if ierr != nil {
			hostname = "[err]"
		} else {
			hostname = info.Hostname
		}

		m.workLk.Lock()
		defer m.workLk.Unlock()

		if err != nil {
			merr := m.work.Get(wk).Mutate(func(ws *WorkState) error {
				ws.Status = wsDone
				ws.WorkError = err.Error()
				return nil
			})

			if merr != nil {
				return xerrors.Errorf("failed to start work and to track the error; merr: %+v, err: %w", merr, err)
			}
			return err
		}

		err = m.work.Get(wk).Mutate(func(ws *WorkState) error {
			_, ok := m.results[wk]
			if ok {
				log.Warn("work returned before we started tracking it")
				ws.Status = wsDone
			} else {
				ws.Status = wsRunning
			}
			ws.WorkerCall = callID
			ws.WorkerHostname = hostname
			ws.StartTime = time.Now().Unix()
			return nil
		})
		if err != nil {
			return xerrors.Errorf("registering running work: %w", err)
		}

		m.callToWork[callID] = wk

		return nil
	}
}

func (m *Manager) waitWork(ctx context.Context, wid WorkID) (interface{}, error) {
	m.workLk.Lock()

	var ws WorkState
	if err := m.work.Get(wid).Get(&ws); err != nil {
		m.workLk.Unlock()
		return nil, xerrors.Errorf("getting work status: %w", err)
	}

	if ws.Status == wsStarted {
		m.workLk.Unlock()
		return nil, xerrors.Errorf("waitWork called for work in 'started' state")
	}

	// sanity check
	wk := m.callToWork[ws.WorkerCall]
	if wk != wid {
		m.workLk.Unlock()
		return nil, xerrors.Errorf("wrong callToWork mapping for call %s; expected %s, got %s", ws.WorkerCall, wid, wk)
	}

	// make sure we don't have the result ready
	cr, ok := m.callRes[ws.WorkerCall]
	if ok {
		delete(m.callToWork, ws.WorkerCall)

		if len(cr) == 1 {
			err := m.work.Get(wk).End()
			if err != nil {
				m.workLk.Unlock()
				// Not great, but not worth discarding potentially multi-hour computation over this
				log.Errorf("marking work as done: %+v", err)
			}

			res := <-cr
			delete(m.callRes, ws.WorkerCall)

			m.workLk.Unlock()
			return res.r, res.err
		}

		m.workLk.Unlock()
		return nil, xerrors.Errorf("something else in waiting on callRes")
	}

	done := func() {
		delete(m.results, wid)

		_, ok := m.callToWork[ws.WorkerCall]
		if ok {
			delete(m.callToWork, ws.WorkerCall)
		}

		err := m.work.Get(wk).End()
		if err != nil {
			// Not great, but not worth discarding potentially multi-hour computation over this
			log.Errorf("marking work as done: %+v", err)
		}
	}

	// the result can already be there if the work was running, manager restarted,
	// and the worker has delivered the result before we entered waitWork
	res, ok := m.results[wid]
	if ok {
		done()
		m.workLk.Unlock()
		return res.r, res.err
	}

	ch, ok := m.waitRes[wid]
	if !ok {
		ch = make(chan struct{})
		m.waitRes[wid] = ch
	}

	m.workLk.Unlock()

	select {
	case <-ch:
		m.workLk.Lock()
		defer m.workLk.Unlock()

		res := m.results[wid]
		done()

		return res.r, res.err
	case <-ctx.Done():
		return nil, xerrors.Errorf("waiting for work result: %w", ctx.Err())
	}
}

func (m *Manager) waitSimpleCall(ctx context.Context) func(callID storiface.CallID, err error) (interface{}, error) {
	return func(callID storiface.CallID, err error) (interface{}, error) {
		if err != nil {
			return nil, err
		}

		return m.waitCall(ctx, callID)
	}
}

func (m *Manager) waitCall(ctx context.Context, callID storiface.CallID) (interface{}, error) {
	m.workLk.Lock()
	_, ok := m.callToWork[callID]
	if ok {
		m.workLk.Unlock()
		return nil, xerrors.Errorf("can't wait for calls related to work")
	}

	ch, ok := m.callRes[callID]
	if !ok {
		ch = make(chan result, 1)
		m.callRes[callID] = ch
	}
	m.workLk.Unlock()

	defer func() {
		m.workLk.Lock()
		defer m.workLk.Unlock()

		delete(m.callRes, callID)
	}()

	select {
	case res := <-ch:
		return res.r, res.err
	case <-ctx.Done():
		return nil, xerrors.Errorf("waiting for call result: %w", ctx.Err())
	}
}

func (m *Manager) returnResult(ctx context.Context, callID storiface.CallID, r interface{}, cerr *storiface.CallError) error {
	res := result{
		r: r,
	}
	if cerr != nil {
		res.err = cerr
	}

	m.sched.workTracker.onDone(ctx, callID)

	m.workLk.Lock()
	defer m.workLk.Unlock()

	wid, ok := m.callToWork[callID]
	if !ok {
		rch, ok := m.callRes[callID]
		if !ok {
			rch = make(chan result, 1)
			m.callRes[callID] = rch
		}

		if len(rch) > 0 {
			return xerrors.Errorf("callRes channel already has a response")
		}
		if cap(rch) == 0 {
			return xerrors.Errorf("expected rch to be buffered")
		}

		rch <- res
		return nil
	}

	_, ok = m.results[wid]
	if ok {
		return xerrors.Errorf("result for call %v already reported", wid)
	}

	m.results[wid] = res

	err := m.work.Get(wid).Mutate(func(ws *WorkState) error {
		ws.Status = wsDone
		return nil
	})
	if err != nil {
		// in the unlikely case:
		// * manager has restarted, and we're still tracking this work, and
		// * the work is abandoned (storage-fsm doesn't do a matching call on the sector), and
		// * the call is returned from the worker, and
		// * this errors
		// the user will get jobs stuck in ret-wait state
		log.Errorf("marking work as done: %+v", err)
	}

	_, found := m.waitRes[wid]
	if found {
		close(m.waitRes[wid])
		delete(m.waitRes, wid)
	}

	return nil
}

func (m *Manager) Abort(ctx context.Context, call storiface.CallID) error {
	// TODO: Allow temp error
	return m.returnResult(ctx, call, nil, storiface.Err(storiface.ErrUnknown, xerrors.New("task aborted")))
}
