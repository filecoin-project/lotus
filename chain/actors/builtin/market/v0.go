package market

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	v0adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type v0State struct {
	market.State
	store adt.Store
}

func (s *v0State) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *v0State) BalancesChanged(otherState State) bool {
	v0otherState, ok := otherState.(*v0State)
	if !ok {
		// there's no way to compare differnt versions of the state, so let's
		// just say that means the state of balances has changed
		return true
	}
	return !s.State.EscrowTable.Equals(v0otherState.State.EscrowTable) || !s.State.LockedTable.Equals(v0otherState.State.LockedTable)
}

func (s *v0State) StatesChanged(otherState State) bool {
	v0otherState, ok := otherState.(*v0State)
	if !ok {
		// there's no way to compare differnt versions of the state, so let's
		// just say that means the state of balances has changed
		return true
	}
	return !s.State.States.Equals(v0otherState.State.States)
}

func (s *v0State) States() (DealStates, error) {
	stateArray, err := v0adt.AsArray(s.store, s.State.States)
	if err != nil {
		return nil, err
	}
	return &v0DealStates{stateArray}, nil
}

func (s *v0State) ProposalsChanged(otherState State) bool {
	v0otherState, ok := otherState.(*v0State)
	if !ok {
		// there's no way to compare differnt versions of the state, so let's
		// just say that means the state of balances has changed
		return true
	}
	return !s.State.Proposals.Equals(v0otherState.State.Proposals)
}

func (s *v0State) Proposals() (DealProposals, error) {
	proposalArray, err := v0adt.AsArray(s.store, s.State.Proposals)
	if err != nil {
		return nil, err
	}
	return &v0DealProposals{proposalArray}, nil
}

func (s *v0State) EscrowTable() (BalanceTable, error) {
	bt, err := v0adt.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &v0BalanceTable{bt}, nil
}

func (s *v0State) LockedTable() (BalanceTable, error) {
	bt, err := v0adt.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &v0BalanceTable{bt}, nil
}

func (s *v0State) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (weight, verifiedWeight abi.DealWeight, err error) {
	return market.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
}

type v0BalanceTable struct {
	*v0adt.BalanceTable
}

func (bt *v0BalanceTable) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*v0adt.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type v0DealStates struct {
	adt.Array
}

func (s *v0DealStates) Get(dealID abi.DealID) (*DealState, bool, error) {
	var v0deal market.DealState
	found, err := s.Array.Get(uint64(dealID), &v0deal)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV0DealState(v0deal)
	return &deal, true, nil
}

func (s *v0DealStates) decode(val *cbg.Deferred) (*DealState, error) {
	var v0ds market.DealState
	if err := v0ds.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV0DealState(v0ds)
	return &ds, nil
}

func (s *v0DealStates) array() adt.Array {
	return s.Array
}

func fromV0DealState(v0 market.DealState) DealState {
	return (DealState)(v0)
}

type v0DealProposals struct {
	adt.Array
}

func (s *v0DealProposals) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var v0proposal market.DealProposal
	found, err := s.Array.Get(uint64(dealID), &v0proposal)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	proposal := fromV0DealProposal(v0proposal)
	return &proposal, true, nil
}

func (s *v0DealProposals) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var v0dp market.DealProposal
	return s.Array.ForEach(&v0dp, func(idx int64) error {
		return cb(abi.DealID(idx), fromV0DealProposal(v0dp))
	})
}

func (s *v0DealProposals) decode(val *cbg.Deferred) (*DealProposal, error) {
	var v0dp market.DealProposal
	if err := v0dp.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	dp := fromV0DealProposal(v0dp)
	return &dp, nil
}

func (s *v0DealProposals) array() adt.Array {
	return s.Array
}

func fromV0DealProposal(v0 market.DealProposal) DealProposal {
	return (DealProposal)(v0)
}
