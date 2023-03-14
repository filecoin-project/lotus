package stmgr

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/trace"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
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
		// This avoids the question of 'who gets paid the genesis block reward'.
		// This also makes us not attempt to lookup the tipset state with
		// tryLookupTipsetState, which would cause a very long, very slow walk.
		return ts.Blocks()[0].ParentStateRoot, ts.Blocks()[0].ParentMessageReceipts, nil
	}

	// First, try to find the tipset in the current chain. If found, we can avoid re-executing
	// it.
	if st, rec, found := tryLookupTipsetState(ctx, sm.cs, ts); found {
		return st, rec, nil
	}

	st, rec, err = sm.tsExec.ExecuteTipSet(ctx, sm, ts, sm.tsExecMonitor, false)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	return st, rec, nil
}

// Try to lookup a state & receipt CID for a given tipset by walking the chain instead of executing
// it. This will only successfully return the state/receipt CIDs if they're found in the state
// store.
//
// NOTE: This _won't_ recursively walk the receipt/state trees. It assumes that having the root
// implies having the rest of the tree. However, lotus generally makes that assumption anyways.
func tryLookupTipsetState(ctx context.Context, cs *store.ChainStore, ts *types.TipSet) (cid.Cid, cid.Cid, bool) {
	nextTs, err := cs.GetTipsetByHeight(ctx, ts.Height()+1, nil, false)
	if err != nil {
		// Nothing to see here. The requested height may be beyond the current head.
		return cid.Undef, cid.Undef, false
	}

	// Make sure we're on the correct fork.
	if nextTs.Parents() != ts.Key() {
		// Also nothing to see here. This just means that the requested tipset is on a
		// different fork.
		return cid.Undef, cid.Undef, false
	}

	stateCid := nextTs.ParentState()
	receiptCid := nextTs.ParentMessageReceipts()

	// Make sure we have the parent state.
	if hasState, err := cs.StateBlockstore().Has(ctx, stateCid); err != nil {
		log.Errorw("failed to lookup state-root in blockstore", "cid", stateCid, "error", err)
		return cid.Undef, cid.Undef, false
	} else if !hasState {
		// We have the chain but don't have the state. It looks like we need to try
		// executing?
		return cid.Undef, cid.Undef, false
	}

	// Make sure we have the receipts.
	if hasReceipts, err := cs.ChainBlockstore().Has(ctx, receiptCid); err != nil {
		log.Errorw("failed to lookup receipts in blockstore", "cid", receiptCid, "error", err)
		return cid.Undef, cid.Undef, false
	} else if !hasReceipts {
		// If we don't have the receipts, re-execute and try again.
		return cid.Undef, cid.Undef, false
	}

	return stateCid, receiptCid, true
}

func (sm *StateManager) ExecutionTraceWithMonitor(ctx context.Context, ts *types.TipSet, em ExecMonitor) (cid.Cid, error) {
	st, _, err := sm.tsExec.ExecuteTipSet(ctx, sm, ts, em, true)
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
