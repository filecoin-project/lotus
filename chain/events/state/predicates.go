package state

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	typegen "github.com/whyrusleeping/cbor-gen"

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

type DiffAdtArraysFunc func(ctx context.Context, oldDealStateRoot, newDealStateRoot *adt.Array) (changed bool, user UserData, err error)

// OnDealStateChanged calls diffDealStates when the market deal state changes
func (sp *StatePredicates) OnDealStateChanged(diffDealStates DiffAdtArraysFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error) {
		if oldState.States.Equals(newState.States) {
			return false, nil, nil
		}

		ctxStore := &contextStore{
			ctx: ctx,
			cst: sp.cst,
		}

		oldRoot, err := adt.AsArray(ctxStore, oldState.States)
		if err != nil {
			return false, nil, err
		}
		newRoot, err := adt.AsArray(ctxStore, newState.States)
		if err != nil {
			return false, nil, err
		}

		return diffDealStates(ctx, oldRoot, newRoot)
	}
}

// OnDealProposalChanged calls diffDealProps when the market proposal state changes
func (sp *StatePredicates) OnDealProposalChanged(diffDealProps DiffAdtArraysFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error) {
		if oldState.Proposals.Equals(newState.Proposals) {
			return false, nil, nil
		}

		ctxStore := &contextStore{
			ctx: ctx,
			cst: sp.cst,
		}

		oldRoot, err := adt.AsArray(ctxStore, oldState.Proposals)
		if err != nil {
			return false, nil, err
		}
		newRoot, err := adt.AsArray(ctxStore, newState.Proposals)
		if err != nil {
			return false, nil, err
		}

		return diffDealProps(ctx, oldRoot, newRoot)
	}
}

var _ AdtArrayDiff = &MarketDealProposalChanges{}

type MarketDealProposalChanges struct {
	Added   []ProposalIDState
	Removed []ProposalIDState
}

type ProposalIDState struct {
	ID       abi.DealID
	Proposal market.DealProposal
}

func (m *MarketDealProposalChanges) Add(key uint64, val *typegen.Deferred) error {
	dp := new(market.DealProposal)
	err := dp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Added = append(m.Added, ProposalIDState{abi.DealID(key), *dp})
	return nil
}

func (m *MarketDealProposalChanges) Modify(key uint64, from, to *typegen.Deferred) error {
	// short circuit, DealProposals are static
	return nil
}

func (m *MarketDealProposalChanges) Remove(key uint64, val *typegen.Deferred) error {
	dp := new(market.DealProposal)
	err := dp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Removed = append(m.Removed, ProposalIDState{abi.DealID(key), *dp})
	return nil
}

// OnDealProposalAmtChanged detects changes in the deal proposal AMT for all deal proposals and returns a MarketProposalsChanges structure containing:
// - Added Proposals
// - Modified Proposals
// - Removed Proposals
func (sp *StatePredicates) OnDealProposalAmtChanged() DiffAdtArraysFunc {
	return func(ctx context.Context, oldDealProps, newDealProps *adt.Array) (changed bool, user UserData, err error) {
		proposalChanges := new(MarketDealProposalChanges)
		if err := DiffAdtArray(oldDealProps, newDealProps, proposalChanges); err != nil {
			return false, nil, err
		}

		if len(proposalChanges.Added)+len(proposalChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, proposalChanges, nil
	}
}

var _ AdtArrayDiff = &MarketDealStateChanges{}

type MarketDealStateChanges struct {
	Added    []DealIDState
	Modified []DealStateChange
	Removed  []DealIDState
}

type DealIDState struct {
	ID   abi.DealID
	Deal market.DealState
}

func (m *MarketDealStateChanges) Add(key uint64, val *typegen.Deferred) error {
	ds := new(market.DealState)
	err := ds.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Added = append(m.Added, DealIDState{abi.DealID(key), *ds})
	return nil
}

func (m *MarketDealStateChanges) Modify(key uint64, from, to *typegen.Deferred) error {
	dsFrom := new(market.DealState)
	if err := dsFrom.UnmarshalCBOR(bytes.NewReader(from.Raw)); err != nil {
		return err
	}

	dsTo := new(market.DealState)
	if err := dsTo.UnmarshalCBOR(bytes.NewReader(to.Raw)); err != nil {
		return err
	}

	if *dsFrom != *dsTo {
		m.Modified = append(m.Modified, DealStateChange{abi.DealID(key), dsFrom, dsTo})
	}
	return nil
}

func (m *MarketDealStateChanges) Remove(key uint64, val *typegen.Deferred) error {
	ds := new(market.DealState)
	err := ds.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Removed = append(m.Removed, DealIDState{abi.DealID(key), *ds})
	return nil
}

// OnDealStateAmtChanged detects changes in the deal state AMT for all deal states and returns a MarketDealStateChanges structure containing:
// - Added Deals
// - Modified Deals
// - Removed Deals
func (sp *StatePredicates) OnDealStateAmtChanged() DiffAdtArraysFunc {
	return func(ctx context.Context, oldDealStates, newDealStates *adt.Array) (changed bool, user UserData, err error) {
		dealStateChanges := new(MarketDealStateChanges)
		if err := DiffAdtArray(oldDealStates, newDealStates, dealStateChanges); err != nil {
			return false, nil, err
		}

		if len(dealStateChanges.Added)+len(dealStateChanges.Modified)+len(dealStateChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, dealStateChanges, nil
	}
}

// ChangedDeals is a set of changes to deal state
type ChangedDeals map[abi.DealID]DealStateChange

// DealStateChange is a change in deal state from -> to
type DealStateChange struct {
	ID   abi.DealID
	From *market.DealState
	To   *market.DealState
}

// DealStateChangedForIDs detects changes in the deal state AMT for the given deal IDs
func (sp *StatePredicates) DealStateChangedForIDs(dealIds []abi.DealID) DiffAdtArraysFunc {
	return func(ctx context.Context, oldDealStateArray, newDealStateArray *adt.Array) (changed bool, user UserData, err error) {
		changedDeals := make(ChangedDeals)
		for _, dealID := range dealIds {
			var oldDealPtr, newDealPtr *market.DealState
			var oldDeal, newDeal market.DealState

			// If the deal has been removed, we just set it to nil
			found, err := oldDealStateArray.Get(uint64(dealID), &oldDeal)
			if err != nil {
				return false, nil, err
			}
			if found {
				oldDealPtr = &oldDeal
			}

			found, err = newDealStateArray.Get(uint64(dealID), &newDeal)
			if err != nil {
				return false, nil, err
			}
			if found {
				newDealPtr = &newDeal
			}

			if oldDeal != newDeal {
				changedDeals[dealID] = DealStateChange{dealID, oldDealPtr, newDealPtr}
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

	if siFrom.Expiration != siTo.Expiration {
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
