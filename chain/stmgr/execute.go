package stmgr

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func (sm *StateManager) TipSetState(ctx context.Context, ts *types.TipSet) (st cid.Cid, rec cid.Cid, err error) {
	ctx, span := trace.StartSpan(ctx, "tipSetState")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(trace.StringAttribute("tipset", fmt.Sprint(ts.Cids())))
	}

	ck := cidsToKey(ts.Cids())
	sm.stlk.Lock()
	cw, cwok := sm.compWait[ck]
	if cwok {
		sm.stlk.Unlock()
		span.AddAttributes(trace.BoolAttribute("waited", true))
		select {
		case <-cw:
			sm.stlk.Lock()
		case <-ctx.Done():
			return cid.Undef, cid.Undef, ctx.Err()
		}
	}
	cached, ok := sm.stCache[ck]
	if ok {
		sm.stlk.Unlock()
		span.AddAttributes(trace.BoolAttribute("cache", true))
		return cached[0], cached[1], nil
	}
	ch := make(chan struct{})
	sm.compWait[ck] = ch

	defer func() {
		sm.stlk.Lock()
		delete(sm.compWait, ck)
		if st != cid.Undef {
			sm.stCache[ck] = []cid.Cid{st, rec}
		}
		sm.stlk.Unlock()
		close(ch)
	}()

	sm.stlk.Unlock()

	if ts.Height() == 0 {
		// NB: This is here because the process that executes blocks requires that the
		// block miner reference a valid miner in the state tree. Unless we create some
		// magical genesis miner, this won't work properly, so we short circuit here
		// This avoids the question of 'who gets paid the genesis block reward'
		return ts.Blocks()[0].ParentStateRoot, ts.Blocks()[0].ParentMessageReceipts, nil
	}

	st, rec, err = sm.tsExec.ExecuteTipSet(ctx, sm, ts, sm.tsExecMonitor)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	return st, rec, nil
}

func (sm *StateManager) ExecutionTraceWithMonitor(ctx context.Context, ts *types.TipSet, em ExecMonitor) (cid.Cid, error) {
	st, _, err := sm.tsExec.ExecuteTipSet(ctx, sm, ts, em)
	return st, err
}

func (sm *StateManager) ExecutionTrace(ctx context.Context, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error) {
	var invocTrace []*api.InvocResult
	st, err := sm.ExecutionTraceWithMonitor(ctx, ts, &InvocationTracer{trace: &invocTrace})
	if err != nil {
		return cid.Undef, nil, err
	}
	return st, invocTrace, nil
}
