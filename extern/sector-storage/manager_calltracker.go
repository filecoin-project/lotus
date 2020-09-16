package sectorstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/xerrors"
	"io"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type workID struct {
	Method string
	Params string // json [...params]
}

func (w *workID) String() string {
	return fmt.Sprintf("%s(%s)", w.Method, w.Params)
}

var _ fmt.Stringer = &workID{}

type WorkStatus string
const (
	wsStarted WorkStatus = "started" // task started, not scheduled/running on a worker yet
	wsRunning WorkStatus = "running" // task running on a worker, waiting for worker return
	wsDone    WorkStatus = "done"    // task returned from the worker, results available
)

type WorkState struct {
	Status WorkStatus

	WorkerCall storiface.CallID // Set when entering wsRunning
	WorkError string // Status = wsDone, set when failed to start work
}

func (w *WorkState) UnmarshalCBOR(reader io.Reader) error {
	panic("implement me")
}

func newWorkID(method string, params ...interface{}) (workID, error) {
	pb, err := json.Marshal(params)
	if err != nil {
		return workID{}, xerrors.Errorf("marshaling work params: %w", err)
	}

	return workID{
		Method: method,
		Params: string(pb),
	}, nil
}

// returns wait=true when the task is already tracked/running
func (m *Manager) getWork(ctx context.Context, method string, params ...interface{}) (wid workID, wait bool, err error) {
	wid, err = newWorkID(method, params)
	if err != nil {
		return workID{}, false, xerrors.Errorf("creating workID: %w", err)
	}

	m.workLk.Lock()
	defer m.workLk.Unlock()

	have, err := m.work.Has(wid)
	if err != nil {
		return workID{}, false, xerrors.Errorf("failed to check if the task is already tracked: %w", err)
	}

	if !have {
		err := m.work.Begin(wid, WorkState{
			Status: wsStarted,
		})
		if err != nil {
			return workID{}, false, xerrors.Errorf("failed to track task start: %w", err)
		}

		return wid, false, nil
	}

	// already started

	return wid, true, nil
}

func (m *Manager) startWork(ctx context.Context, wk workID) func(callID storiface.CallID, err error) error {
	return func(callID storiface.CallID, err error) error {
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
			return nil
		})
		if err != nil {
			return xerrors.Errorf("registering running work: %w", err)
		}

		m.callToWork[callID] = wk

		return nil
	}
}

func (m *Manager) waitWork(ctx context.Context, wid workID) (interface{}, error) {
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

			res := <- cr
			delete(m.callRes, ws.WorkerCall)

			m.workLk.Unlock()
			return res.r, res.err
		}

		m.workLk.Unlock()
		return nil, xerrors.Errorf("something else in waiting on callRes")
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
		delete(m.results, wid)

		err := m.work.Get(wk).End()
		if err != nil {
			// Not great, but not worth discarding potentially multi-hour computation over this
			log.Errorf("marking work as done: %+v", err)
		}

		return res.r, res.err
	case <-ctx.Done():
		return nil, xerrors.Errorf("waiting for work result: %w", ctx.Err())
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
		ch = make(chan result)
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

func (m *Manager) returnResult(callID storiface.CallID, r interface{}, serr string) error {
	var err error
	if serr != "" {
		err = errors.New(serr)
	}

	res := result{
		r:   r,
		err: err,
	}

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
		return xerrors.Errorf("result for call %v already reported")
	}

	m.results[wid] = res

	close(m.waitRes[wid])
	delete(m.waitRes, wid)

	return nil
}
