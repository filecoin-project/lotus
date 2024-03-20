package state

import (
	"context"

	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
)

// UserData is the data returned from the DiffTipSetKeyFunc
type UserData interface{}

// ChainAPI abstracts out calls made by this class to external APIs
type ChainAPI interface {
	api.ChainIO
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
		cst: cbor.NewCborStore(blockstore.NewAPIBlockstore(api)),
	}
}

// DiffTipSetKeyFunc check if there's a change form oldState to newState, and returns
// - changed: was there a change
// - user: user-defined data representing the state change
// - err
type DiffTipSetKeyFunc func(ctx context.Context, oldState, newState types.TipSetKey) (changed bool, user UserData, err error)

type DiffActorStateFunc func(ctx context.Context, oldActorState *types.Actor, newActorState *types.Actor) (changed bool, user UserData, err error)

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
		return diffStateFunc(ctx, oldActor, newActor)
	}
}

type DiffStorageMarketStateFunc func(ctx context.Context, oldState market.State, newState market.State) (changed bool, user UserData, err error)

// OnStorageMarketActorChanged calls diffStorageMarketState when the state changes for the market actor
func (sp *StatePredicates) OnStorageMarketActorChanged(diffStorageMarketState DiffStorageMarketStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(market.Address, func(ctx context.Context, oldActorState, newActorState *types.Actor) (changed bool, user UserData, err error) {
		oldState, err := market.Load(adt.WrapStore(ctx, sp.cst), oldActorState)
		if err != nil {
			return false, nil, err
		}
		newState, err := market.Load(adt.WrapStore(ctx, sp.cst), newActorState)
		if err != nil {
			return false, nil, err
		}
		return diffStorageMarketState(ctx, oldState, newState)
	})
}

type BalanceTables struct {
	EscrowTable market.BalanceTable
	LockedTable market.BalanceTable
}

// DiffBalanceTablesFunc compares two balance tables
type DiffBalanceTablesFunc func(ctx context.Context, oldBalanceTable, newBalanceTable BalanceTables) (changed bool, user UserData, err error)

// OnBalanceChanged runs when the escrow table for available balances changes
func (sp *StatePredicates) OnBalanceChanged(diffBalances DiffBalanceTablesFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState market.State, newState market.State) (changed bool, user UserData, err error) {
		bc, err := oldState.BalancesChanged(newState)
		if err != nil {
			return false, nil, err
		}

		if !bc {
			return false, nil, nil
		}

		oldEscrowRoot, err := oldState.EscrowTable()
		if err != nil {
			return false, nil, err
		}

		oldLockedRoot, err := oldState.LockedTable()
		if err != nil {
			return false, nil, err
		}

		newEscrowRoot, err := newState.EscrowTable()
		if err != nil {
			return false, nil, err
		}

		newLockedRoot, err := newState.LockedTable()
		if err != nil {
			return false, nil, err
		}

		return diffBalances(ctx, BalanceTables{oldEscrowRoot, oldLockedRoot}, BalanceTables{newEscrowRoot, newLockedRoot})
	}
}

type DiffDealStatesFunc func(ctx context.Context, oldDealStateRoot, newDealStateRoot market.DealStates) (changed bool, user UserData, err error)
type DiffDealProposalsFunc func(ctx context.Context, oldDealStateRoot, newDealStateRoot market.DealProposals) (changed bool, user UserData, err error)
type DiffAdtArraysFunc func(ctx context.Context, oldDealStateRoot, newDealStateRoot adt.Array) (changed bool, user UserData, err error)

// OnDealStateChanged calls diffDealStates when the market deal state changes
func (sp *StatePredicates) OnDealStateChanged(diffDealStates DiffDealStatesFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState market.State, newState market.State) (changed bool, user UserData, err error) {
		sc, err := oldState.StatesChanged(newState)
		if err != nil {
			return false, nil, err
		}

		if !sc {
			return false, nil, nil
		}

		oldRoot, err := oldState.States()
		if err != nil {
			return false, nil, err
		}
		newRoot, err := newState.States()
		if err != nil {
			return false, nil, err
		}

		return diffDealStates(ctx, oldRoot, newRoot)
	}
}

// OnDealProposalChanged calls diffDealProps when the market proposal state changes
func (sp *StatePredicates) OnDealProposalChanged(diffDealProps DiffDealProposalsFunc) DiffStorageMarketStateFunc {
	return func(ctx context.Context, oldState market.State, newState market.State) (changed bool, user UserData, err error) {
		pc, err := oldState.ProposalsChanged(newState)
		if err != nil {
			return false, nil, err
		}

		if !pc {
			return false, nil, nil
		}

		oldRoot, err := oldState.Proposals()
		if err != nil {
			return false, nil, err
		}
		newRoot, err := newState.Proposals()
		if err != nil {
			return false, nil, err
		}

		return diffDealProps(ctx, oldRoot, newRoot)
	}
}

// OnDealProposalAmtChanged detects changes in the deal proposal AMT for all deal proposals and returns a MarketProposalsChanges structure containing:
// - Added Proposals
// - Modified Proposals
// - Removed Proposals
func (sp *StatePredicates) OnDealProposalAmtChanged() DiffDealProposalsFunc {
	return func(ctx context.Context, oldDealProps, newDealProps market.DealProposals) (changed bool, user UserData, err error) {
		proposalChanges, err := market.DiffDealProposals(oldDealProps, newDealProps)
		if err != nil {
			return false, nil, err
		}

		if len(proposalChanges.Added)+len(proposalChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, proposalChanges, nil
	}
}

// OnDealStateAmtChanged detects changes in the deal state AMT for all deal states and returns a MarketDealStateChanges structure containing:
// - Added Deals
// - Modified Deals
// - Removed Deals
func (sp *StatePredicates) OnDealStateAmtChanged() DiffDealStatesFunc {
	return func(ctx context.Context, oldDealStates, newDealStates market.DealStates) (changed bool, user UserData, err error) {
		dealStateChanges, err := market.DiffDealStates(oldDealStates, newDealStates)
		if err != nil {
			return false, nil, err
		}

		if len(dealStateChanges.Added)+len(dealStateChanges.Modified)+len(dealStateChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, dealStateChanges, nil
	}
}

// ChangedDeals is a set of changes to deal state
type ChangedDeals map[abi.DealID]market.DealStateChange

// DealStateChangedForIDs detects changes in the deal state AMT for the given deal IDs
func (sp *StatePredicates) DealStateChangedForIDs(dealIds []abi.DealID) DiffDealStatesFunc {
	return func(ctx context.Context, oldDealStates, newDealStates market.DealStates) (changed bool, user UserData, err error) {
		changedDeals := make(ChangedDeals)
		for _, dealID := range dealIds {

			// If the deal has been removed, we just set it to nil
			oldDeal, oldFound, err := oldDealStates.Get(dealID)
			if err != nil {
				return false, nil, err
			}

			newDeal, newFound, err := newDealStates.Get(dealID)
			if err != nil {
				return false, nil, err
			}

			existenceChanged := oldFound != newFound
			valueChanged := (oldFound && newFound) && !oldDeal.Equals(newDeal)
			if existenceChanged || valueChanged {
				changedDeals[dealID] = market.DealStateChange{ID: dealID, From: oldDeal, To: newDeal}
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

type DiffMinerActorStateFunc func(ctx context.Context, oldState miner.State, newState miner.State) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnInitActorChange(diffInitActorState DiffInitActorStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(init_.Address, func(ctx context.Context, oldActorState, newActorState *types.Actor) (changed bool, user UserData, err error) {
		oldState, err := init_.Load(adt.WrapStore(ctx, sp.cst), oldActorState)
		if err != nil {
			return false, nil, err
		}
		newState, err := init_.Load(adt.WrapStore(ctx, sp.cst), newActorState)
		if err != nil {
			return false, nil, err
		}
		return diffInitActorState(ctx, oldState, newState)
	})

}

func (sp *StatePredicates) OnMinerActorChange(minerAddr address.Address, diffMinerActorState DiffMinerActorStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(minerAddr, func(ctx context.Context, oldActorState, newActorState *types.Actor) (changed bool, user UserData, err error) {
		oldState, err := miner.Load(adt.WrapStore(ctx, sp.cst), oldActorState)
		if err != nil {
			return false, nil, err
		}
		newState, err := miner.Load(adt.WrapStore(ctx, sp.cst), newActorState)
		if err != nil {
			return false, nil, err
		}
		return diffMinerActorState(ctx, oldState, newState)
	})
}

func (sp *StatePredicates) OnMinerSectorChange() DiffMinerActorStateFunc {
	return func(ctx context.Context, oldState, newState miner.State) (changed bool, user UserData, err error) {
		sectorChanges, err := miner.DiffSectors(oldState, newState)
		if err != nil {
			return false, nil, err
		}
		// nothing changed
		if len(sectorChanges.Added)+len(sectorChanges.Extended)+len(sectorChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, sectorChanges, nil
	}
}

func (sp *StatePredicates) OnMinerPreCommitChange() DiffMinerActorStateFunc {
	return func(ctx context.Context, oldState, newState miner.State) (changed bool, user UserData, err error) {
		precommitChanges, err := miner.DiffPreCommits(oldState, newState)
		if err != nil {
			return false, nil, err
		}

		if len(precommitChanges.Added)+len(precommitChanges.Removed) == 0 {
			return false, nil, nil
		}

		return true, precommitChanges, nil
	}
}

// DiffPaymentChannelStateFunc is function that compares two states for the payment channel
type DiffPaymentChannelStateFunc func(ctx context.Context, oldState paych.State, newState paych.State) (changed bool, user UserData, err error)

// OnPaymentChannelActorChanged calls diffPaymentChannelState when the state changes for the payment channel actor
func (sp *StatePredicates) OnPaymentChannelActorChanged(paychAddr address.Address, diffPaymentChannelState DiffPaymentChannelStateFunc) DiffTipSetKeyFunc {
	return sp.OnActorStateChanged(paychAddr, func(ctx context.Context, oldActorState, newActorState *types.Actor) (changed bool, user UserData, err error) {
		oldState, err := paych.Load(adt.WrapStore(ctx, sp.cst), oldActorState)
		if err != nil {
			return false, nil, err
		}
		newState, err := paych.Load(adt.WrapStore(ctx, sp.cst), newActorState)
		if err != nil {
			return false, nil, err
		}
		return diffPaymentChannelState(ctx, oldState, newState)
	})
}

// PayChToSendChange is a difference in the amount to send on a payment channel when the money is collected
type PayChToSendChange struct {
	OldToSend abi.TokenAmount
	NewToSend abi.TokenAmount
}

// OnToSendAmountChanges monitors changes on the total amount to send from one party to the other on a payment channel
func (sp *StatePredicates) OnToSendAmountChanges() DiffPaymentChannelStateFunc {
	return func(ctx context.Context, oldState paych.State, newState paych.State) (changed bool, user UserData, err error) {
		ots, err := oldState.ToSend()
		if err != nil {
			return false, nil, err
		}

		nts, err := newState.ToSend()
		if err != nil {
			return false, nil, err
		}

		if ots.Equals(nts) {
			return false, nil, nil
		}
		return true, &PayChToSendChange{
			OldToSend: ots,
			NewToSend: nts,
		}, nil
	}
}

type AddressPair struct {
	ID address.Address
	PK address.Address
}

type DiffInitActorStateFunc func(ctx context.Context, oldState init_.State, newState init_.State) (changed bool, user UserData, err error)

func (sp *StatePredicates) OnAddressMapChange() DiffInitActorStateFunc {
	return func(ctx context.Context, oldState, newState init_.State) (changed bool, user UserData, err error) {
		addressChanges, err := init_.DiffAddressMap(oldState, newState)
		if err != nil {
			return false, nil, err
		}
		if len(addressChanges.Added)+len(addressChanges.Modified)+len(addressChanges.Removed) == 0 {
			return false, nil, nil
		}
		return true, addressChanges, nil
	}
}
