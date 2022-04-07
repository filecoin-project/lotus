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

	market4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/market"
	adt4 "github.com/filecoin-project/specs-actors/v4/actors/util/adt"
)

var _ State = (*state4)(nil)

func load4(store adt.Store, root cid.Cid) (State, error) {
	out := state4{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make4(store adt.Store) (State, error) {
	out := state4{store: store}

	s, err := market4.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state4 struct {
	market4.State
	store adt.Store
}

func (s *state4) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state4) BalancesChanged(otherState State) (bool, error) {
	otherState4, ok := otherState.(*state4)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState4.State.EscrowTable) || !s.State.LockedTable.Equals(otherState4.State.LockedTable), nil
}

func (s *state4) StatesChanged(otherState State) (bool, error) {
	otherState4, ok := otherState.(*state4)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState4.State.States), nil
}

func (s *state4) States() (DealStates, error) {
	stateArray, err := adt4.AsArray(s.store, s.State.States, market4.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates4{stateArray}, nil
}

func (s *state4) ProposalsChanged(otherState State) (bool, error) {
	otherState4, ok := otherState.(*state4)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState4.State.Proposals), nil
}

func (s *state4) Proposals() (DealProposals, error) {
	proposalArray, err := adt4.AsArray(s.store, s.State.Proposals, market4.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals4{proposalArray}, nil
}

func (s *state4) EscrowTable() (BalanceTable, error) {
	bt, err := adt4.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable4{bt}, nil
}

func (s *state4) LockedTable() (BalanceTable, error) {
	bt, err := adt4.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable4{bt}, nil
}

func (s *state4) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (weight, verifiedWeight abi.DealWeight, err error) {
	w, vw, _, err := market4.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return w, vw, err
}

func (s *state4) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable4 struct {
	*adt4.BalanceTable
}

func (bt *balanceTable4) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt4.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates4 struct {
	adt.Array
}

func (s *dealStates4) Get(dealID abi.DealID) (*DealState, bool, error) {
	var deal4 market4.DealState
	found, err := s.Array.Get(uint64(dealID), &deal4)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV4DealState(deal4)
	return &deal, true, nil
}

func (s *dealStates4) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds4 market4.DealState
	return s.Array.ForEach(&ds4, func(idx int64) error {
		return cb(abi.DealID(idx), fromV4DealState(ds4))
	})
}

func (s *dealStates4) decode(val *cbg.Deferred) (*DealState, error) {
	var ds4 market4.DealState
	if err := ds4.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV4DealState(ds4)
	return &ds, nil
}

func (s *dealStates4) array() adt.Array {
	return s.Array
}

func fromV4DealState(v4 market4.DealState) DealState {
	return (DealState)(v4)
}

type dealProposals4 struct {
	adt.Array
}

func (s *dealProposals4) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal4 market4.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal4)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	proposal := fromV4DealProposal(proposal4)
	return &proposal, true, nil
}

func (s *dealProposals4) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp4 market4.DealProposal
	return s.Array.ForEach(&dp4, func(idx int64) error {
		return cb(abi.DealID(idx), fromV4DealProposal(dp4))
	})
}

func (s *dealProposals4) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp4 market4.DealProposal
	if err := dp4.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	dp := fromV4DealProposal(dp4)
	return &dp, nil
}

func (s *dealProposals4) array() adt.Array {
	return s.Array
}

func fromV4DealProposal(v4 market4.DealProposal) DealProposal {
	return (DealProposal)(v4)
}

func (s *state4) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn4)(nil)

func decodePublishStorageDealsReturn4(b []byte) (PublishStorageDealsReturn, error) {
	var retval market4.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn4{retval}, nil
}

type publishStorageDealsReturn4 struct {
	market4.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn4) IsDealValid(index uint64) (bool, error) {

	// PublishStorageDeals only succeeded if all deals were valid in this version of actors
	return true, nil

}

func (r *publishStorageDealsReturn4) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}
