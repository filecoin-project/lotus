package state

import (
	"context"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/ipfs/go-cid"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
)

// Data returned from the DiffFunc
type UserData interface{}

// The calls made by this class external APIs
type ChainApi interface {
	apibstore.ChainIO
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
}

// Use StatePredicates to respond to state changes
type StatePredicates struct {
	api ChainApi
	cst *cbor.BasicIpldStore
}

func NewStatePredicates(api ChainApi) *StatePredicates {
	return &StatePredicates{
		api: api,
		cst: cbor.NewCborStore(apibstore.NewAPIBlockstore(api)),
	}
}

// Check if there's a change form oldState to newState, and return
// - changed: was there a change
// - user: user-defined data representing the state change
// - err
type DiffFunc func(ctx context.Context, oldState, newState *types.TipSet) (changed bool, user UserData, err error)

type DiffStateFunc func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error)

// Calls diffStateFunc when the state changes for the given actor
func (sp *StatePredicates) OnActorStateChanged(addr address.Address, diffStateFunc DiffStateFunc) DiffFunc {
	return func(ctx context.Context, oldState, newState *types.TipSet) (changed bool, user UserData, err error) {
		oldActor, err := sp.api.StateGetActor(ctx, addr, oldState.Key())
		if err != nil {
			return false, nil, err
		}
		newActor, err := sp.api.StateGetActor(ctx, addr, newState.Key())
		if oldActor.Head.Equals(newActor.Head) {
			return false, nil, nil
		}
		return diffStateFunc(ctx, oldActor.Head, newActor.Head)
	}
}

type DiffStorageMarketStateFunc func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error)

// Calls diffStorageMarketState when the state changes for the market actor
func (sp *StatePredicates) OnStorageMarketActorChanged(diffStorageMarketState DiffStorageMarketStateFunc) DiffFunc {
	return sp.OnActorStateChanged(builtin.StorageMarketActorAddr, func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error) {
		var oldState market.State
		if err := sp.cst.Get(ctx, oldActorStateHead, &oldState); err != nil {
			return false, nil, err
		}
		var newState market.State
		if err := sp.cst.Get(ctx, newActorStateHead, &newState); err != nil {
			return false, nil, err
		}
		return diffStorageMarketState(ctx, &oldState, &newState)
	})
}

type DiffDealStatesFunc func(ctx context.Context, oldDealStateRoot *amt.Root, newDealStateRoot *amt.Root) (changed bool, user UserData, err error)

// Calls diffDealStates when the market state changes
func (sp *StatePredicates) OnDealStateChanged(diffDealStates DiffDealStatesFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error) {
		if oldState.States.Equals(newState.States) {
			return false, nil, nil
		}

		oldRoot, err := amt.LoadAMT(ctx, sp.cst, oldState.States)
		if err != nil {
			return false, nil, err
		}
		newRoot, err := amt.LoadAMT(ctx, sp.cst, newState.States)
		if err != nil {
			return false, nil, err
		}

		return diffDealStates(ctx, oldRoot, newRoot)
	}
}

// A set of changes to deal state
type ChangedDeals map[abi.DealID]DealStateChange

// Change in deal state from -> to
type DealStateChange struct {
	From market.DealState
	To market.DealState
}

// Detect changes in the deal state AMT for the given deal IDs
func (sp *StatePredicates) DealStateChangedForIDs(dealIds []abi.DealID) DiffDealStatesFunc {
	return func(ctx context.Context, oldDealStateRoot *amt.Root, newDealStateRoot *amt.Root) (changed bool, user UserData, err error) {
		changedDeals := make(ChangedDeals)
		for _, dealId := range dealIds {
			var oldDeal, newDeal market.DealState
			err := oldDealStateRoot.Get(ctx, uint64(dealId), &oldDeal)
			if err != nil {
				return false, nil, err
			}
			err = newDealStateRoot.Get(ctx, uint64(dealId), &newDeal)
			if err != nil {
				return false, nil, err
			}
			if oldDeal != newDeal {
				changedDeals[dealId] = DealStateChange{oldDeal, newDeal}
			}
		}
		if len(changedDeals) > 0 {
			return true, changedDeals, nil
		}
		return false, nil, nil
	}
}
