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

type UserData interface{}

type ChainApi interface {
	ChainHasObj(context.Context, cid.Cid) (bool, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
}

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

type DiffFunc func(ctx context.Context, oldState, newState *types.TipSet) (changed bool, user UserData, err error)

type DiffStateFunc func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error)

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

func (sp *StatePredicates) OnStorageMarketActorChanged(addr address.Address, diffStorageMarketState DiffStorageMarketStateFunc) DiffFunc {
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

func (sp *StatePredicates) DealStateChangedForIDs(dealIds []abi.DealID) DiffDealStatesFunc {
	return func(ctx context.Context, oldDealStateRoot *amt.Root, newDealStateRoot *amt.Root) (changed bool, user UserData, err error) {
		var changedDeals []abi.DealID
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
				changedDeals = append(changedDeals, dealId)
			}
		}
		if len(changedDeals) > 0 {
			return true, changed, nil
		}
		return false, nil, nil
	}
}
