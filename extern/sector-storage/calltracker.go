package sectorstorage

import (
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

type callTracker struct {
	st *statestore.StateStore // by CallID
}

type CallState uint64

const (
	CallStarted CallState = iota
	CallDone
	// returned -> remove
)

type Call struct {
	State CallState

	// Params cbg.Deferred // TODO: support once useful
	Result []byte
}

func (wt *callTracker) onStart(ci storiface.CallID) error {
	return wt.st.Begin(ci, &Call{
		State: CallStarted,
	})
}

func (wt *callTracker) onDone(ci storiface.CallID, ret []byte) error {
	st := wt.st.Get(ci)
	return st.Mutate(func(cs *Call) error {
		cs.State = CallDone
		cs.Result = ret
		return nil
	})
}

func (wt *callTracker) onReturned(ci storiface.CallID) error {
	st := wt.st.Get(ci)
	return st.End()
}
