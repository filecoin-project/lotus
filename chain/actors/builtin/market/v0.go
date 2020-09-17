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
	return v0adt.AsBalanceTable(s.store, s.State.EscrowTable)
}

func (s *v0State) LockedTable() (BalanceTable, error) {
	return v0adt.AsBalanceTable(s.store, s.State.LockedTable)
}

func (s *v0State) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (weight, verifiedWeight abi.DealWeight, err error) {
	return market.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
}

type v0DealStates struct {
	adt.Array
}

func (s *v0DealStates) GetDeal(dealID abi.DealID) (DealState, error) {
	var deal market.DealState
	found, err := s.Array.Get(uint64(dealID), &deal)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &v0DealState{deal}, nil
}

func (s *v0DealStates) Diff(other DealStates) (*DealStateChanges, error) {
	v0other, ok := other.(*v0DealStates)
	if !ok {
		// TODO handle this if possible on a case by case basis but for now, just fail
		return nil, errors.New("cannot compare deal states across versions")
	}
	results := new(DealStateChanges)
	if err := adt.DiffAdtArray(s, v0other, &v0MarketStatesDiffer{results}); err != nil {
		return nil, fmt.Errorf("diffing deal states: %w", err)
	}

	return results, nil
}

type v0MarketStatesDiffer struct {
	Results *DealStateChanges
}

func (d *v0MarketStatesDiffer) Add(key uint64, val *typegen.Deferred) error {
	ds := new(v0DealState)
	err := ds.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Added = append(d.Results.Added, DealIDState{abi.DealID(key), ds})
	return nil
}

func (d *v0MarketStatesDiffer) Modify(key uint64, from, to *typegen.Deferred) error {
	dsFrom := new(v0DealState)
	if err := dsFrom.UnmarshalCBOR(bytes.NewReader(from.Raw)); err != nil {
		return err
	}

	dsTo := new(v0DealState)
	if err := dsTo.UnmarshalCBOR(bytes.NewReader(to.Raw)); err != nil {
		return err
	}

	if *dsFrom != *dsTo {
		d.Results.Modified = append(d.Results.Modified, DealStateChange{abi.DealID(key), dsFrom, dsTo})
	}
	return nil
}

func (d *v0MarketStatesDiffer) Remove(key uint64, val *typegen.Deferred) error {
	ds := new(v0DealState)
	err := ds.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Removed = append(d.Results.Removed, DealIDState{abi.DealID(key), ds})
	return nil
}

type v0DealState struct {
	market.DealState
}

func (ds *v0DealState) SectorStartEpoch() abi.ChainEpoch {
	return ds.DealState.SectorStartEpoch
}

func (ds *v0DealState) SlashEpoch() abi.ChainEpoch {
	return ds.DealState.SlashEpoch
}

func (ds *v0DealState) LastUpdatedEpoch() abi.ChainEpoch {
	return ds.DealState.LastUpdatedEpoch
}

func (ds *v0DealState) Equals(other DealState) bool {
	v0other, ok := other.(*v0DealState)
	return ok && *ds == *v0other
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
	if err := adt.DiffAdtArray(s, v0other, &v0MarketProposalsDiffer{results}); err != nil {
		return nil, fmt.Errorf("diffing deal proposals: %w", err)
	}

	return results, nil
}

type v0MarketProposalsDiffer struct {
	Results *DealProposalChanges
}

type v0DealProposal struct {
	market.DealProposal
}

func (d *v0MarketProposalsDiffer) Add(key uint64, val *typegen.Deferred) error {
	dp := new(v0DealProposal)
	err := dp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Added = append(d.Results.Added, ProposalIDState{abi.DealID(key), dp})
	return nil
}

func (d *v0MarketProposalsDiffer) Modify(key uint64, from, to *typegen.Deferred) error {
	// short circuit, DealProposals are static
	return nil
}

func (d *v0MarketProposalsDiffer) Remove(key uint64, val *typegen.Deferred) error {
	dp := new(v0DealProposal)
	err := dp.UnmarshalCBOR(bytes.NewReader(val.Raw))
	if err != nil {
		return err
	}
	d.Results.Removed = append(d.Results.Removed, ProposalIDState{abi.DealID(key), dp})
	return nil
}
