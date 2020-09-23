package market

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	adt1 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
	cbg "github.com/whyrusleeping/cbor-gen"
)

var _ State = (*state1)(nil)

type state1 struct {
	market.State
	store adt.Store
}

func (s *state1) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state1) BalancesChanged(otherState State) (bool, error) {
	otherState1, ok := otherState.(*state1)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState1.State.EscrowTable) || !s.State.LockedTable.Equals(otherState1.State.LockedTable), nil
}

func (s *state1) StatesChanged(otherState State) (bool, error) {
	otherState1, ok := otherState.(*state1)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState1.State.States), nil
}

func (s *state1) States() (DealStates, error) {
	stateArray, err := adt1.AsArray(s.store, s.State.States)
	if err != nil {
		return nil, err
	}
	return &dealStates1{stateArray}, nil
}

func (s *state1) ProposalsChanged(otherState State) (bool, error) {
	otherState1, ok := otherState.(*state1)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState1.State.Proposals), nil
}

func (s *state1) Proposals() (DealProposals, error) {
	proposalArray, err := adt1.AsArray(s.store, s.State.Proposals)
	if err != nil {
		return nil, err
	}
	return &dealProposals1{proposalArray}, nil
}

func (s *state1) EscrowTable() (BalanceTable, error) {
	bt, err := adt1.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable1{bt}, nil
}

func (s *state1) LockedTable() (BalanceTable, error) {
	bt, err := adt1.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable1{bt}, nil
}

func (s *state1) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (weight, verifiedWeight abi.DealWeight, err error) {
	w, vw, _, err := market.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return w, vw, err
}

type balanceTable1 struct {
	*adt1.BalanceTable
}

func (bt *balanceTable1) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt1.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates1 struct {
	adt.Array
}

func (s *dealStates1) Get(dealID abi.DealID) (*DealState, bool, error) {
	var deal1 market.DealState
	found, err := s.Array.Get(uint64(dealID), &deal1)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV1DealState(deal1)
	return &deal, true, nil
}

func (s *dealStates1) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds1 market.DealState
	return s.Array.ForEach(&ds1, func(idx int64) error {
		return cb(abi.DealID(idx), fromV1DealState(ds1))
	})
}

func (s *dealStates1) decode(val *cbg.Deferred) (*DealState, error) {
	var ds1 market.DealState
	if err := ds1.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV1DealState(ds1)
	return &ds, nil
}

func (s *dealStates1) array() adt.Array {
	return s.Array
}

func fromV1DealState(v1 market.DealState) DealState {
	return (DealState)(v1)
}

type dealProposals1 struct {
	adt.Array
}

func (s *dealProposals1) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal1 market.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal1)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	proposal := fromV1DealProposal(proposal1)
	return &proposal, true, nil
}

func (s *dealProposals1) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp1 market.DealProposal
	return s.Array.ForEach(&dp1, func(idx int64) error {
		return cb(abi.DealID(idx), fromV1DealProposal(dp1))
	})
}

func (s *dealProposals1) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp1 market.DealProposal
	if err := dp1.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	dp := fromV1DealProposal(dp1)
	return &dp, nil
}

func (s *dealProposals1) array() adt.Array {
	return s.Array
}

func fromV1DealProposal(v1 market.DealProposal) DealProposal {
	return (DealProposal)(v1)
}
