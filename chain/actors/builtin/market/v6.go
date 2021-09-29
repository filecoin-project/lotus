package market

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"

	market6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/market"
	adt6 "github.com/filecoin-project/specs-actors/v6/actors/util/adt"
)

var _ State = (*state6)(nil)

func load6(store adt.Store, root cid.Cid) (State, error) {
	out := state6{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make6(store adt.Store) (State, error) {
	out := state6{store: store}

	s, err := market6.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state6 struct {
	market6.State
	store adt.Store
}

func (s *state6) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state6) BalancesChanged(otherState State) (bool, error) {
	otherState6, ok := otherState.(*state6)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState6.State.EscrowTable) || !s.State.LockedTable.Equals(otherState6.State.LockedTable), nil
}

func (s *state6) StatesChanged(otherState State) (bool, error) {
	otherState6, ok := otherState.(*state6)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState6.State.States), nil
}

func (s *state6) States() (DealStates, error) {
	stateArray, err := adt6.AsArray(s.store, s.State.States, market6.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates6{stateArray}, nil
}

func (s *state6) ProposalsChanged(otherState State) (bool, error) {
	otherState6, ok := otherState.(*state6)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState6.State.Proposals), nil
}

func (s *state6) Proposals() (DealProposals, error) {
	proposalArray, err := adt6.AsArray(s.store, s.State.Proposals, market6.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals6{proposalArray}, nil
}

func (s *state6) EscrowTable() (BalanceTable, error) {
	bt, err := adt6.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable6{bt}, nil
}

func (s *state6) LockedTable() (BalanceTable, error) {
	bt, err := adt6.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable6{bt}, nil
}

func (s *state6) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (weight, verifiedWeight abi.DealWeight, err error) {
	w, vw, _, err := market6.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return w, vw, err
}

func (s *state6) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable6 struct {
	*adt6.BalanceTable
}

func (bt *balanceTable6) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt6.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates6 struct {
	adt.Array
}

func (s *dealStates6) Get(dealID abi.DealID) (*DealState, bool, error) {
	var deal6 market6.DealState
	found, err := s.Array.Get(uint64(dealID), &deal6)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV6DealState(deal6)
	return &deal, true, nil
}

func (s *dealStates6) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds6 market6.DealState
	return s.Array.ForEach(&ds6, func(idx int64) error {
		return cb(abi.DealID(idx), fromV6DealState(ds6))
	})
}

func (s *dealStates6) decode(val *cbg.Deferred) (*DealState, error) {
	var ds6 market6.DealState
	if err := ds6.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV6DealState(ds6)
	return &ds, nil
}

func (s *dealStates6) array() adt.Array {
	return s.Array
}

func fromV6DealState(v6 market6.DealState) DealState {
	return (DealState)(v6)
}

type dealProposals6 struct {
	adt.Array
}

func (s *dealProposals6) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal6 market6.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal6)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	proposal := fromV6DealProposal(proposal6)
	return &proposal, true, nil
}

func (s *dealProposals6) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp6 market6.DealProposal
	return s.Array.ForEach(&dp6, func(idx int64) error {
		return cb(abi.DealID(idx), fromV6DealProposal(dp6))
	})
}

func (s *dealProposals6) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp6 market6.DealProposal
	if err := dp6.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	dp := fromV6DealProposal(dp6)
	return &dp, nil
}

func (s *dealProposals6) array() adt.Array {
	return s.Array
}

func fromV6DealProposal(v6 market6.DealProposal) DealProposal {
	return (DealProposal)(v6)
}

func (s *state6) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn6)(nil)

func decodePublishStorageDealsReturn6(b []byte) (PublishStorageDealsReturn, error) {
	var retval market6.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn6{retval}, nil
}

type publishStorageDealsReturn6 struct {
	market6.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn6) IsDealValid(index uint64) (bool, error) {

	return r.ValidDeals.IsSet(index)

}

func (r *publishStorageDealsReturn6) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}
