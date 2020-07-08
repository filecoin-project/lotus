package state

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	typegen "github.com/whyrusleeping/cbor-gen"

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

var _ AdtArrayDiff = &MinerSectorChanges{}

type SectorExtensions struct {
	From miner.SectorOnChainInfo
	To   miner.SectorOnChainInfo
}

func (m *MinerSectorChanges) Add(key uint64, val *typegen.Deferred) error {
	si := new(miner.SectorOnChainInfo)
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Added = append(m.Added, *si)
	return nil
}

func (m *MinerSectorChanges) Modify(key uint64, from, to *typegen.Deferred) error {
	siFrom := new(miner.SectorOnChainInfo)
	err := siFrom.UnmarshalCBOR(bytes.NewReader(from.Raw))
	if err != nil {
		return err
	}

	siTo := new(miner.SectorOnChainInfo)
	err = siTo.UnmarshalCBOR(bytes.NewReader(to.Raw))
	if err != nil {
		return err
	}

	if siFrom.Info.Expiration != siTo.Info.Expiration {
		m.Extended = append(m.Extended, SectorExtensions{
			From: *siFrom,
			To:   *siTo,
		})
	}
	return nil
}

func (m *MinerSectorChanges) Remove(key uint64, val *typegen.Deferred) error {
	si := new(miner.SectorOnChainInfo)
	err := si.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Removed = append(m.Removed, *si)
	return nil
}

func (sp *StatePredicates) OnMinerSectorChange() DiffMinerActorStateFunc {
	return func(ctx context.Context, oldState, newState *miner.State) (changed bool, user UserData, err error) {
		ctxStore := &contextStore{
			ctx: ctx,
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

		if err := DiffAdtArray(oldSectors, newSectors, sectorChanges); err != nil {
			return false, nil, err
		}

		// nothing changed
		if len(sectorChanges.Added)+len(sectorChanges.Extended)+len(sectorChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, sectorChanges, nil
	}
}
