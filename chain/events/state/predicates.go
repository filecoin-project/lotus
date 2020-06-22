package state

import (
	"context"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

type StatePredicates struct {
	sm *stmgr.StateManager
}

func NewStatePredicates(sm *stmgr.StateManager) *StatePredicates {
	return &StatePredicates{
		sm: sm,
	}
}

type DiffStateFunc func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnActorStateChanged(addr address.Address, diffStateFunc DiffStateFunc) DiffFunc {
	return func(ctx context.Context, oldState, newState *types.TipSet) (changed bool, user UserData, err error) {
		oldActor, err := sp.sm.GetActor(addr, oldState)
		if err != nil {
			return false, nil, err
		}
		newActor, err := sp.sm.GetActor(addr, newState)
		if oldActor.Head.Equals(newActor.Head) {
			return false, nil, nil
		}
		return diffStateFunc(ctx, oldActor.Head, newActor.Head)
	}
}

type DiffStorageMarketStateFunc func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnStorageMarketActorChanged(addr address.Address, diffStorageMarketState DiffStorageMarketStateFunc) DiffFunc {
	return sp.OnActorStateChanged(builtin.StorageMarketActorAddr, func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error) {
		var oldState market.State
		cst := cbor.NewCborStore(sp.sm.ChainStore().Blockstore())
		if err := cst.Get(ctx, oldActorStateHead, &oldState); err != nil {
			return false, nil, err
		}
		var newState market.State
		if err := cst.Get(ctx, newActorStateHead, &newActorStateHead); err != nil {
			return false, nil, err
		}
		return diffStorageMarketState(ctx, &oldState, &newState)
	})
}

type DiffDealStatesFunc func(ctx context.Context, oldDealStateRoot *amt.Root, newDealStateRoot *amt.Root) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnDealStateChanged(diffDealStates DiffDealStatesFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error) {
		if oldState.States.Equals(newState.States) {
			return false, nil, nil
		}
		blks := cbor.NewCborStore(sp.sm.ChainStore().Blockstore())
		oldRoot, err := amt.LoadAMT(ctx, blks, oldState.States)
		if err != nil {
			return false, nil, err
		}
		newRoot, err := amt.LoadAMT(ctx, blks, newState.States)
		if err != nil {
			return false, nil, err
		}
		return diffDealStates(ctx, oldRoot, newRoot)
	}
}

func (sp *StatePredicates) DealStateChangedForIDs(ctx context.Context, dealIds []abi.DealID, oldRoot, newRoot *amt.Root) (changed bool, user UserData, err error) {
	var changedDeals []abi.DealID
	for _, dealId := range dealIds {
		var oldDeal, newDeal market.DealState
		err := oldRoot.Get(ctx, uint64(dealId), &oldDeal)
		if err != nil {
			return false, nil, err
		}
		err = newRoot.Get(ctx, uint64(dealId), &newDeal)
		if err != nil {
			return false, nil, err
		}
		if oldDeal != newDeal {
			changedDeals = append(changedDeals, dealId)
		}
	}
	if len(changedDeals) > 0 {
		return true, changed, nil
	}
	return false, nil, nil
}
