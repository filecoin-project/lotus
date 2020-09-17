package market

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	v0adt "github.com/filecoin-project/specs-actors/actors/util/adt"
	typegen "github.com/whyrusleeping/cbor-gen"
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

func (s *v0DealStates) Diff(other DealStates) (*DealStateChanges, error) {
	v0other, ok := other.(*v0DealStates)
	if !ok {
		// TODO handle this if possible on a case by case basis but for now, just fail
		return nil, errors.New("cannot compare deal states across versions")
	}
	results := new(DealStateChanges)
	if err := adt.DiffAdtArray(s.Array, v0other.Array, &v0MarketStatesDiffer{results}); err != nil {
		return nil, fmt.Errorf("diffing deal states: %w", err)
	}

	return results, nil
}

type v0MarketStatesDiffer struct {
	Results *DealStateChanges
}

func (d *v0MarketStatesDiffer) Add(key uint64, val *typegen.Deferred) error {
	v0ds := new(market.DealState)
	err := v0ds.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Added = append(d.Results.Added, DealIDState{abi.DealID(key), fromV0DealState(*v0ds)})
	return nil
}

func (d *v0MarketStatesDiffer) Modify(key uint64, from, to *typegen.Deferred) error {
	v0dsFrom := new(market.DealState)
	if err := v0dsFrom.UnmarshalCBOR(bytes.NewReader(from.Raw)); err != nil {
		return err
	}

	v0dsTo := new(market.DealState)
	if err := v0dsTo.UnmarshalCBOR(bytes.NewReader(to.Raw)); err != nil {
		return err
	}

	if *v0dsFrom != *v0dsTo {
		dsFrom := fromV0DealState(*v0dsFrom)
		dsTo := fromV0DealState(*v0dsTo)
		d.Results.Modified = append(d.Results.Modified, DealStateChange{abi.DealID(key), &dsFrom, &dsTo})
	}
	return nil
}

func (d *v0MarketStatesDiffer) Remove(key uint64, val *typegen.Deferred) error {
	v0ds := new(market.DealState)
	err := v0ds.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Removed = append(d.Results.Removed, DealIDState{abi.DealID(key), fromV0DealState(*v0ds)})
	return nil
}

func fromV0DealState(v0 market.DealState) DealState {
	return DealState{
		SectorStartEpoch: v0.SectorStartEpoch,
		SlashEpoch:       v0.SlashEpoch,
		LastUpdatedEpoch: v0.LastUpdatedEpoch,
	}
}

type v0DealProposals struct {
	adt.Array
}

func (s *v0DealProposals) Diff(other DealProposals) (*DealProposalChanges, error) {
	v0other, ok := other.(*v0DealProposals)
	if !ok {
		// TODO handle this if possible on a case by case basis but for now, just fail
		return nil, errors.New("cannot compare deal proposals across versions")
	}
	results := new(DealProposalChanges)
	if err := adt.DiffAdtArray(s.Array, v0other.Array, &v0MarketProposalsDiffer{results}); err != nil {
		return nil, fmt.Errorf("diffing deal proposals: %w", err)
	}

	return results, nil
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

type v0MarketProposalsDiffer struct {
	Results *DealProposalChanges
}

func fromV0DealProposal(v0 market.DealProposal) DealProposal {
	return DealProposal{
		PieceCID:             v0.PieceCID,
		PieceSize:            v0.PieceSize,
		VerifiedDeal:         v0.VerifiedDeal,
		Client:               v0.Client,
		Provider:             v0.Provider,
		Label:                v0.Label,
		StartEpoch:           v0.StartEpoch,
		EndEpoch:             v0.EndEpoch,
		StoragePricePerEpoch: v0.StoragePricePerEpoch,
		ProviderCollateral:   v0.ProviderCollateral,
		ClientCollateral:     v0.ClientCollateral,
	}
}

func (d *v0MarketProposalsDiffer) Add(key uint64, val *typegen.Deferred) error {
	v0dp := new(market.DealProposal)
	err := v0dp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Added = append(d.Results.Added, ProposalIDState{abi.DealID(key), fromV0DealProposal(*v0dp)})
	return nil
}

func (d *v0MarketProposalsDiffer) Modify(key uint64, from, to *typegen.Deferred) error {
	// short circuit, DealProposals are static
	return nil
}

func (d *v0MarketProposalsDiffer) Remove(key uint64, val *typegen.Deferred) error {
	v0dp := new(market.DealProposal)
	err := v0dp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Removed = append(d.Results.Removed, ProposalIDState{abi.DealID(key), fromV0DealProposal(*v0dp)})
	return nil
}
