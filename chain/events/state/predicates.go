package state

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	typegen "github.com/whyrusleeping/cbor-gen"

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

type BalanceTables struct {
	EscrowTable *adt.BalanceTable
	LockedTable *adt.BalanceTable
}

// DiffBalanceTablesFunc compares two balance tables
type DiffBalanceTablesFunc func(ctx context.Context, oldBalanceTable, newBalanceTable BalanceTables) (changed bool, user UserData, err error)

// OnBalanceChanged runs when the escrow table for available balances changes
func (sp *StatePredicates) OnBalanceChanged(diffBalances DiffBalanceTablesFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState *market.State, newState *market.State) (changed bool, user UserData, err error) {
		if oldState.EscrowTable.Equals(newState.EscrowTable) && oldState.LockedTable.Equals(newState.LockedTable) {
			return false, nil, nil
		}

		ctxStore := &contextStore{
			ctx: ctx,
			cst: sp.cst,
		}

		oldEscrowRoot, err := adt.AsBalanceTable(ctxStore, oldState.EscrowTable)
		if err != nil {
			return false, nil, err
		}

		oldLockedRoot, err := adt.AsBalanceTable(ctxStore, oldState.LockedTable)
		if err != nil {
			return false, nil, err
		}

		newEscrowRoot, err := adt.AsBalanceTable(ctxStore, newState.EscrowTable)
		if err != nil {
			return false, nil, err
		}

		newLockedRoot, err := adt.AsBalanceTable(ctxStore, newState.LockedTable)
		if err != nil {
			return false, nil, err
		}

		return diffBalances(ctx, BalanceTables{oldEscrowRoot, oldLockedRoot}, BalanceTables{newEscrowRoot, newLockedRoot})
	}
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

// ChangedBalances is a set of changes to deal state
type ChangedBalances map[address.Address]BalanceChange

// BalanceChange is a change in balance from -> to
type BalanceChange struct {
	From abi.TokenAmount
	To   abi.TokenAmount
}

// AvailableBalanceChangedForAddresses detects changes in the escrow table for the given addresses
func (sp *StatePredicates) AvailableBalanceChangedForAddresses(getAddrs func() []address.Address) DiffBalanceTablesFunc {
	return func(ctx context.Context, oldBalances, newBalances BalanceTables) (changed bool, user UserData, err error) {
		changedBalances := make(ChangedBalances)
		addrs := getAddrs()
		for _, addr := range addrs {
			// If the deal has been removed, we just set it to nil
			oldEscrowBalance, err := oldBalances.EscrowTable.Get(addr)
			if err != nil {
				return false, nil, err
			}

			oldLockedBalance, err := oldBalances.LockedTable.Get(addr)
			if err != nil {
				return false, nil, err
			}

			oldBalance := big.Sub(oldEscrowBalance, oldLockedBalance)

			newEscrowBalance, err := newBalances.EscrowTable.Get(addr)
			if err != nil {
				return false, nil, err
			}

			newLockedBalance, err := newBalances.LockedTable.Get(addr)
			if err != nil {
				return false, nil, err
			}

			newBalance := big.Sub(newEscrowBalance, newLockedBalance)

			if !oldBalance.Equals(newBalance) {
				changedBalances[addr] = BalanceChange{oldBalance, newBalance}
			}
		}
		if len(changedBalances) > 0 {
			return true, changedBalances, nil
		}
		return false, nil, nil
	}
}

type DiffMinerActorStateFunc func(ctx context.Context, oldState *miner.State, newState *miner.State) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnInitActorChange(diffInitActorState DiffInitActorStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(builtin.InitActorAddr, func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error) {
		var oldState init_.State
		if err := sp.cst.Get(ctx, oldActorStateHead, &oldState); err != nil {
			return false, nil, err
		}
		var newState init_.State
		if err := sp.cst.Get(ctx, newActorStateHead, &newState); err != nil {
			return false, nil, err
		}
		return diffInitActorState(ctx, &oldState, &newState)
	})

}

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

type MinerPreCommitChanges struct {
	Added   []miner.SectorPreCommitOnChainInfo
	Removed []miner.SectorPreCommitOnChainInfo
}

func (m *MinerPreCommitChanges) AsKey(key string) (adt.Keyer, error) {
	sector, err := adt.ParseUIntKey(key)
	if err != nil {
		return nil, err
	}
	return miner.SectorKey(abi.SectorNumber(sector)), nil
}

func (m *MinerPreCommitChanges) Add(key string, val *typegen.Deferred) error {
	sp := new(miner.SectorPreCommitOnChainInfo)
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Added = append(m.Added, *sp)
	return nil
}

func (m *MinerPreCommitChanges) Modify(key string, from, to *typegen.Deferred) error {
	return nil
}

func (m *MinerPreCommitChanges) Remove(key string, val *typegen.Deferred) error {
	sp := new(miner.SectorPreCommitOnChainInfo)
	err := sp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	m.Removed = append(m.Removed, *sp)
	return nil
}

func (sp *StatePredicates) OnMinerPreCommitChange() DiffMinerActorStateFunc {
	return func(ctx context.Context, oldState, newState *miner.State) (changed bool, user UserData, err error) {
		ctxStore := &contextStore{
			ctx: ctx,
			cst: sp.cst,
		}

		precommitChanges := &MinerPreCommitChanges{
			Added:   []miner.SectorPreCommitOnChainInfo{},
			Removed: []miner.SectorPreCommitOnChainInfo{},
		}

		if oldState.PreCommittedSectors.Equals(newState.PreCommittedSectors) {
			return false, nil, nil
		}

		oldPrecommits, err := adt.AsMap(ctxStore, oldState.PreCommittedSectors)
		if err != nil {
			return false, nil, err
		}

		newPrecommits, err := adt.AsMap(ctxStore, newState.PreCommittedSectors)
		if err != nil {
			return false, nil, err
		}

		if err := DiffAdtMap(oldPrecommits, newPrecommits, precommitChanges); err != nil {
			return false, nil, err
		}

		if len(precommitChanges.Added)+len(precommitChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, precommitChanges, nil
	}
}

// DiffPaymentChannelStateFunc is function that compares two states for the payment channel
type DiffPaymentChannelStateFunc func(ctx context.Context, oldState *paych.State, newState *paych.State) (changed bool, user UserData, err error)

// OnPaymentChannelActorChanged calls diffPaymentChannelState when the state changes for the the payment channel actor
func (sp *StatePredicates) OnPaymentChannelActorChanged(paychAddr address.Address, diffPaymentChannelState DiffPaymentChannelStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(paychAddr, func(ctx context.Context, oldActorStateHead, newActorStateHead cid.Cid) (changed bool, user UserData, err error) {
		var oldState paych.State
		if err := sp.cst.Get(ctx, oldActorStateHead, &oldState); err != nil {
			return false, nil, err
		}
		var newState paych.State
		if err := sp.cst.Get(ctx, newActorStateHead, &newState); err != nil {
			return false, nil, err
		}
		return diffPaymentChannelState(ctx, &oldState, &newState)
	})
}

// PayChToSendChange is a difference in the amount to send on a payment channel when the money is collected
type PayChToSendChange struct {
	OldToSend abi.TokenAmount
	NewToSend abi.TokenAmount
}

// OnToSendAmountChanges monitors changes on the total amount to send from one party to the other on a payment channel
func (sp *StatePredicates) OnToSendAmountChanges() DiffPaymentChannelStateFunc {
	return func(ctx context.Context, oldState *paych.State, newState *paych.State) (changed bool, user UserData, err error) {
		if oldState.ToSend.Equals(newState.ToSend) {
			return false, nil, nil
		}
		return true, &PayChToSendChange{
			OldToSend: oldState.ToSend,
			NewToSend: newState.ToSend,
		}, nil
	}
}

type AddressPair struct {
	ID address.Address
	PK address.Address
}

type InitActorAddressChanges struct {
	Added    []AddressPair
	Modified []AddressChange
	Removed  []AddressPair
}

type AddressChange struct {
	From AddressPair
	To   AddressPair
}

type DiffInitActorStateFunc func(ctx context.Context, oldState *init_.State, newState *init_.State) (changed bool, user UserData, err error)

func (i *InitActorAddressChanges) AsKey(key string) (adt.Keyer, error) {
	addr, err := address.NewFromBytes([]byte(key))
	if err != nil {
		return nil, err
	}
	return adt.AddrKey(addr), nil
}

func (i *InitActorAddressChanges) Add(key string, val *typegen.Deferred) error {
	pkAddr, err := address.NewFromBytes([]byte(key))
	if err != nil {
		return err
	}
	id := new(typegen.CborInt)
	if err := id.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return err
	}
	idAddr, err := address.NewIDAddress(uint64(*id))
	if err != nil {
		return err
	}
	i.Added = append(i.Added, AddressPair{
		ID: idAddr,
		PK: pkAddr,
	})
	return nil
}

func (i *InitActorAddressChanges) Modify(key string, from, to *typegen.Deferred) error {
	pkAddr, err := address.NewFromBytes([]byte(key))
	if err != nil {
		return err
	}

	fromID := new(typegen.CborInt)
	if err := fromID.UnmarshalCBOR(bytes.NewReader(from.Raw)); err != nil {
		return err
	}
	fromIDAddr, err := address.NewIDAddress(uint64(*fromID))
	if err != nil {
		return err
	}

	toID := new(typegen.CborInt)
	if err := toID.UnmarshalCBOR(bytes.NewReader(to.Raw)); err != nil {
		return err
	}
	toIDAddr, err := address.NewIDAddress(uint64(*toID))
	if err != nil {
		return err
	}

	i.Modified = append(i.Modified, AddressChange{
		From: AddressPair{
			ID: fromIDAddr,
			PK: pkAddr,
		},
		To: AddressPair{
			ID: toIDAddr,
			PK: pkAddr,
		},
	})
	return nil
}

func (i *InitActorAddressChanges) Remove(key string, val *typegen.Deferred) error {
	pkAddr, err := address.NewFromBytes([]byte(key))
	if err != nil {
		return err
	}
	id := new(typegen.CborInt)
	if err := id.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return err
	}
	idAddr, err := address.NewIDAddress(uint64(*id))
	if err != nil {
		return err
	}
	i.Removed = append(i.Removed, AddressPair{
		ID: idAddr,
		PK: pkAddr,
	})
	return nil
}

func (sp *StatePredicates) OnAddressMapChange() DiffInitActorStateFunc {
	return func(ctx context.Context, oldState, newState *init_.State) (changed bool, user UserData, err error) {
		ctxStore := &contextStore{
			ctx: ctx,
			cst: sp.cst,
		}

		addressChanges := &InitActorAddressChanges{
			Added:    []AddressPair{},
			Modified: []AddressChange{},
			Removed:  []AddressPair{},
		}

		if oldState.AddressMap.Equals(newState.AddressMap) {
			return false, nil, nil
		}

		oldAddrs, err := adt.AsMap(ctxStore, oldState.AddressMap)
		if err != nil {
			return false, nil, err
		}

		newAddrs, err := adt.AsMap(ctxStore, newState.AddressMap)
		if err != nil {
			return false, nil, err
		}

		if err := DiffAdtMap(oldAddrs, newAddrs, addressChanges); err != nil {
			return false, nil, err
		}

		if len(addressChanges.Added)+len(addressChanges.Removed)+len(addressChanges.Modified) == 0 {
			return false, nil, nil
		}

		return true, addressChanges, nil
	}
}
