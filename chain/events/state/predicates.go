package state

import (
	"context"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v2"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/types"
)

// UserData is the data returned from the DiffTipSetKeyFunc
type UserData interface{}

// ChainAPI abstracts out calls made by this class to external APIs
type ChainAPI interface {
	apibstore.ChainIO
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
}

// StatePredicates has common predicates for responding to state changes
type StatePredicates struct {
	api ChainAPI
	cst *cbor.BasicIpldStore
}

func NewStatePredicates(api ChainAPI) *StatePredicates {
	return &StatePredicates{
		api: api,
		cst: cbor.NewCborStore(apibstore.NewAPIBlockstore(api)),
	}
}

// DiffTipSetKeyFunc check if there's a change form oldState to newState, and returns
// - changed: was there a change
// - user: user-defined data representing the state change
// - err
type DiffTipSetKeyFunc func(ctx context.Context, oldState, newState types.TipSetKey) (changed bool, user UserData, err error)

type DiffActorStateFunc func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error)

// OnActorStateChanged calls diffStateFunc when the state changes for the given actor
func (sp *StatePredicates) OnActorStateChanged(addr address.Address, diffStateFunc DiffActorStateFunc) DiffTipSetKeyFunc {
	return func(ctx context.Context, oldState, newState types.TipSetKey) (changed bool, user UserData, err error) {
		oldActor, err := sp.api.StateGetActor(ctx, addr, oldState)
		if err != nil {
			return false, nil, err
		}
		newActor, err := sp.api.StateGetActor(ctx, addr, newState)
		if err != nil {
			return false, nil, err
		}

		if oldActor.Head.Equals(newActor.Head) {
			return false, nil, nil
		}
		return diffStateFunc(ctx, oldActor.Head, newActor.Head)
	}
}

type DiffStorageMarketStateFunc func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error)

// OnStorageMarketActorChanged calls diffStorageMarketState when the state changes for the market actor
func (sp *StatePredicates) OnStorageMarketActorChanged(diffStorageMarketState DiffStorageMarketStateFunc) DiffTipSetKeyFunc {
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

// OnDealStateChanged calls diffDealStates when the market state changes
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

// ChangedDeals is a set of changes to deal state
type ChangedDeals map[abi.DealID]DealStateChange

// DealStateChange is a change in deal state from -> to
type DealStateChange struct {
	From market.DealState
	To   market.DealState
}

// DealStateChangedForIDs detects changes in the deal state AMT for the given deal IDs
func (sp *StatePredicates) DealStateChangedForIDs(dealIds []abi.DealID) DiffDealStatesFunc {
	return func(ctx context.Context, oldDealStateRoot *amt.Root, newDealStateRoot *amt.Root) (changed bool, user UserData, err error) {
		changedDeals := make(ChangedDeals)
		for _, dealID := range dealIds {
			var oldDeal, newDeal market.DealState
			err := oldDealStateRoot.Get(ctx, uint64(dealID), &oldDeal)
			if err != nil {
				return false, nil, err
			}
			err = newDealStateRoot.Get(ctx, uint64(dealID), &newDeal)
			if err != nil {
				return false, nil, err
			}
			if oldDeal != newDeal {
				changedDeals[dealID] = DealStateChange{oldDeal, newDeal}
			}
		}
		if len(changedDeals) > 0 {
			return true, changedDeals, nil
		}
		return false, nil, nil
	}
}

type DiffMinerActorStateFunc func(ctx context.Context, oldState *miner.State, newState *miner.State) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnMinerActorChange(minerAddr address.Address, diffMinerActorState DiffMinerActorStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(minerAddr, func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error) {
		var oldState miner.State
		if err := sp.cst.Get(ctx, oldActorStateHead, &oldState); err != nil {
			return false, nil, err
		}
		var newState miner.State
		if err := sp.cst.Get(ctx, newActorStateHead, &newState); err != nil {
			return false, nil, err
		}
		return diffMinerActorState(ctx, &oldState, &newState)
	})
}

type MinerSectorChanges struct {
	Added    []miner.SectorOnChainInfo
	Extended []SectorExtensions
	Removed  []miner.SectorOnChainInfo
}

type SectorExtensions struct {
	From miner.SectorOnChainInfo
	To   miner.SectorOnChainInfo
}

func (sp *StatePredicates) OnMinerSectorChange() DiffMinerActorStateFunc {
	return func(ctx context.Context, oldState, newState *miner.State) (changed bool, user UserData, err error) {
		ctxStore := &contextStore{
			ctx: context.TODO(),
			cst: sp.cst,
		}

		sectorChanges := &MinerSectorChanges{
			Added:    []miner.SectorOnChainInfo{},
			Extended: []SectorExtensions{},
			Removed:  []miner.SectorOnChainInfo{},
		}

		// no sector changes
		if oldState.Sectors.Equals(newState.Sectors) {
			return false, nil, nil
		}

		oldSectors, err := adt.AsArray(ctxStore, oldState.Sectors)
		if err != nil {
			return false, nil, err
		}

		newSectors, err := adt.AsArray(ctxStore, newState.Sectors)
		if err != nil {
			return false, nil, err
		}

		var osi miner.SectorOnChainInfo

		// find all sectors that were extended or removed
		if err := oldSectors.ForEach(&osi, func(i int64) error {
			var nsi miner.SectorOnChainInfo
			found, err := newSectors.Get(uint64(osi.Info.SectorNumber), &nsi)
			if err != nil {
				return err
			}
			if !found {
				sectorChanges.Removed = append(sectorChanges.Removed, osi)
				return nil
			}

			if nsi.Info.Expiration != osi.Info.Expiration {
				sectorChanges.Extended = append(sectorChanges.Extended, SectorExtensions{
					From: osi,
					To:   nsi,
				})
			}

			// we don't update miners state filed with `newSectors.Root()` so this operation is safe.
			if err := newSectors.Delete(uint64(osi.Info.SectorNumber)); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return false, nil, err
		}

		// all sectors that remain in newSectors are new
		var nsi miner.SectorOnChainInfo
		if err := newSectors.ForEach(&nsi, func(i int64) error {
			sectorChanges.Added = append(sectorChanges.Added, nsi)
			return nil
		}); err != nil {
			return false, nil, err
		}

		// nothing changed
		if len(sectorChanges.Added)+len(sectorChanges.Extended)+len(sectorChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, sectorChanges, nil
	}
}

type contextStore struct {
	ctx context.Context
	cst *cbor.BasicIpldStore
}

func (cs *contextStore) Context() context.Context {
	return cs.ctx
}

func (cs *contextStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	return cs.cst.Get(ctx, c, out)
}

func (cs *contextStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return cs.cst.Put(ctx, v)
}
