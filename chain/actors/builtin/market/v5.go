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

	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"
	adt5 "github.com/filecoin-project/specs-actors/v5/actors/util/adt"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store) (State, error) {
	out := state5{store: store}

	s, err := market5.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state5 struct {
	market5.State
	store adt.Store
}

func (s *state5) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state5) BalancesChanged(otherState State) (bool, error) {
	otherState5, ok := otherState.(*state5)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState5.State.EscrowTable) || !s.State.LockedTable.Equals(otherState5.State.LockedTable), nil
}

func (s *state5) StatesChanged(otherState State) (bool, error) {
	otherState5, ok := otherState.(*state5)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState5.State.States), nil
}

func (s *state5) States() (DealStates, error) {
	stateArray, err := adt5.AsArray(s.store, s.State.States, market5.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates5{stateArray}, nil
}

func (s *state5) ProposalsChanged(otherState State) (bool, error) {
	otherState5, ok := otherState.(*state5)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState5.State.Proposals), nil
}

func (s *state5) Proposals() (DealProposals, error) {
	proposalArray, err := adt5.AsArray(s.store, s.State.Proposals, market5.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals5{proposalArray}, nil
}

func (s *state5) EscrowTable() (BalanceTable, error) {
	bt, err := adt5.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable5{bt}, nil
}

func (s *state5) LockedTable() (BalanceTable, error) {
	bt, err := adt5.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable5{bt}, nil
}

func (s *state5) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (weight, verifiedWeight abi.DealWeight, err error) {
	w, vw, _, err := market5.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return w, vw, err
}

func (s *state5) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable5 struct {
	*adt5.BalanceTable
}

func (bt *balanceTable5) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt5.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates5 struct {
	adt.Array
}

func (s *dealStates5) Get(dealID abi.DealID) (*DealState, bool, error) {
	var deal5 market5.DealState
	found, err := s.Array.Get(uint64(dealID), &deal5)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV5DealState(deal5)
	return &deal, true, nil
}

func (s *dealStates5) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds5 market5.DealState
	return s.Array.ForEach(&ds5, func(idx int64) error {
		return cb(abi.DealID(idx), fromV5DealState(ds5))
	})
}

func (s *dealStates5) decode(val *cbg.Deferred) (*DealState, error) {
	var ds5 market5.DealState
	if err := ds5.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV5DealState(ds5)
	return &ds, nil
}

func (s *dealStates5) array() adt.Array {
	return s.Array
}

func fromV5DealState(v5 market5.DealState) DealState {
	return (DealState)(v5)
}

type dealProposals5 struct {
	adt.Array
}

func (s *dealProposals5) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal5 market5.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal5)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	proposal := fromV5DealProposal(proposal5)
	return &proposal, true, nil
}

func (s *dealProposals5) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp5 market5.DealProposal
	return s.Array.ForEach(&dp5, func(idx int64) error {
		return cb(abi.DealID(idx), fromV5DealProposal(dp5))
	})
}

func (s *dealProposals5) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp5 market5.DealProposal
	if err := dp5.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	dp := fromV5DealProposal(dp5)
	return &dp, nil
}

func (s *dealProposals5) array() adt.Array {
	return s.Array
}

func fromV5DealProposal(v5 market5.DealProposal) DealProposal {
	return (DealProposal)(v5)
}

func (s *state5) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn5)(nil)

func decodePublishStorageDealsReturn5(b []byte) (PublishStorageDealsReturn, error) {
	var retval market5.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn5{retval}, nil
}

type publishStorageDealsReturn5 struct {
	market5.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn5) IsDealValid(index uint64) (bool, error) {

	// PublishStorageDeals only succeeded if all deals were valid in this version of actors
	return true, nil

}

func (r *publishStorageDealsReturn5) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}
