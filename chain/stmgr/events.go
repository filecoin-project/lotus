package stmgr

import (
	"context"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type stateMgr interface {
	GetActor(address.Address, *types.TipSet) (*types.Actor, error)
	GetReceipt(context.Context, cid.Cid, *types.TipSet) (*types.MessageReceipt, error)
}

func NewEvents(ctx context.Context, chain *store.ChainStore, sm stateMgr) *events.Events {
	return events.NewEvents(ctx, &eventsAdapter{
		chain: chain,
		sm: sm,
	})
}

type eventsAdapter struct {
	chain *store.ChainStore
	sm    stateMgr
}

func (e *eventsAdapter) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	return e.chain.SubHeadChanges(ctx), nil
}

func (e *eventsAdapter) ChainGetBlockMessages(ctx context.Context, c cid.Cid) (*api.BlockMessages, error) {
	b, err := e.chain.GetBlock(c)
	if err != nil {
		return nil, err
	}

	bmsgs, smsgs, err := e.chain.MessagesForBlock(b)
	if err != nil {
		return nil, err
	}

	cids := make([]cid.Cid, len(bmsgs)+len(smsgs))

	for i, m := range bmsgs {
		cids[i] = m.Cid()
	}

	for i, m := range smsgs {
		cids[i+len(bmsgs)] = m.Cid()
	}

	return &api.BlockMessages{
		BlsMessages:   bmsgs,
		SecpkMessages: smsgs,
		Cids:          cids,
	}, nil
}

func (e *eventsAdapter) ChainGetTipSetByHeight(ctx context.Context, h abi.ChainEpoch, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, err := e.chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, err
	}
	return e.chain.GetTipsetByHeight(ctx, h, ts, true)
}

func (e *eventsAdapter) StateGetReceipt(ctx context.Context, c cid.Cid, tsk types.TipSetKey) (*types.MessageReceipt, error) {
	ts, err := e.chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, err
	}
	return e.sm.GetReceipt(ctx, c, ts)
}

func (e *eventsAdapter) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return e.chain.GetTipSetFromKey(tsk)
}

func (e *eventsAdapter) StateGetActor(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	ts, err := e.chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, err
	}
	return e.sm.GetActor(addr, ts)
}
