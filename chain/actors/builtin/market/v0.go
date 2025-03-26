package market

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state0)(nil)

func load0(store adt.Store, root cid.Cid) (State, error) {
	out := state0{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make0(store adt.Store) (State, error) {
	out := state0{store: store}

	ea, err := adt0.MakeEmptyArray(store).Root()
	if err != nil {
		return nil, err
	}

	em, err := adt0.MakeEmptyMap(store).Root()
	if err != nil {
		return nil, err
	}

	out.State = *market0.ConstructState(ea, em, em)

	return &out, nil
}

type state0 struct {
	market0.State
	store adt.Store
}

func (s *state0) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state0) BalancesChanged(otherState State) (bool, error) {
	otherState0, ok := otherState.(*state0)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState0.State.EscrowTable) || !s.State.LockedTable.Equals(otherState0.State.LockedTable), nil
}

func (s *state0) StatesChanged(otherState State) (bool, error) {
	otherState0, ok := otherState.(*state0)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState0.State.States), nil
}

func (s *state0) States() (DealStates, error) {
	stateArray, err := adt0.AsArray(s.store, s.State.States)
	if err != nil {
		return nil, err
	}
	return &dealStates0{stateArray}, nil
}

func (s *state0) ProposalsChanged(otherState State) (bool, error) {
	otherState0, ok := otherState.(*state0)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState0.State.Proposals), nil
}

func (s *state0) Proposals() (DealProposals, error) {
	proposalArray, err := adt0.AsArray(s.store, s.State.Proposals)
	if err != nil {
		return nil, err
	}
	return &dealProposals0{proposalArray}, nil
}

func (s *state0) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt0.AsSet(s.store, s.State.PendingProposals)
	if err != nil {
		return nil, err
	}
	return &pendingProposals0{proposalCidSet}, nil
}

func (s *state0) EscrowTable() (BalanceTable, error) {
	bt, err := adt0.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable0{bt}, nil
}

func (s *state0) LockedTable() (BalanceTable, error) {
	bt, err := adt0.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable0{bt}, nil
}

func (s *state0) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, err := market0.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state0) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable0 struct {
	*adt0.BalanceTable
}

func (bt *balanceTable0) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt0.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates0 struct {
	adt.Array
}

func (s *dealStates0) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal0 market0.DealState
	found, err := s.Array.Get(uint64(dealID), &deal0)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV0DealState(deal0)
	return deal, true, nil
}

func (s *dealStates0) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds0 market0.DealState
	return s.Array.ForEach(&ds0, func(idx int64) error {
		return cb(abi.DealID(idx), fromV0DealState(ds0))
	})
}

func (s *dealStates0) decode(val *cbg.Deferred) (DealState, error) {
	var ds0 market0.DealState
	if err := ds0.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV0DealState(ds0)
	return ds, nil
}

func (s *dealStates0) array() adt.Array {
	return s.Array
}

type dealStateV0 struct {
	ds0 market0.DealState
}

func (d dealStateV0) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV0) SectorStartEpoch() abi.ChainEpoch {
	return d.ds0.SectorStartEpoch
}

func (d dealStateV0) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds0.LastUpdatedEpoch
}

func (d dealStateV0) SlashEpoch() abi.ChainEpoch {
	return d.ds0.SlashEpoch
}

func (d dealStateV0) Equals(other DealState) bool {
	if ov0, ok := other.(dealStateV0); ok {
		return d.ds0 == ov0.ds0
	}

	if d.SectorStartEpoch() != other.SectorStartEpoch() {
		return false
	}
	if d.LastUpdatedEpoch() != other.LastUpdatedEpoch() {
		return false
	}
	if d.SlashEpoch() != other.SlashEpoch() {
		return false
	}

	return true
}

var _ DealState = (*dealStateV0)(nil)

func fromV0DealState(v0 market0.DealState) DealState {
	return dealStateV0{v0}
}

type dealProposals0 struct {
	adt.Array
}

func (s *dealProposals0) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal0 market0.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal0)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV0DealProposal(proposal0)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals0) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp0 market0.DealProposal
	return s.Array.ForEach(&dp0, func(idx int64) error {
		dp, err := fromV0DealProposal(dp0)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals0) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp0 market0.DealProposal
	if err := dp0.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV0DealProposal(dp0)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals0) array() adt.Array {
	return s.Array
}

type pendingProposals0 struct {
	*adt0.Set
}

func (s *pendingProposals0) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV0DealProposal(v0 market0.DealProposal) (DealProposal, error) {

	label, err := labelFromGoString(v0.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v0.PieceCID,
		PieceSize:    v0.PieceSize,
		VerifiedDeal: v0.VerifiedDeal,
		Client:       v0.Client,
		Provider:     v0.Provider,

		Label: label,

		StartEpoch:           v0.StartEpoch,
		EndEpoch:             v0.EndEpoch,
		StoragePricePerEpoch: v0.StoragePricePerEpoch,

		ProviderCollateral: v0.ProviderCollateral,
		ClientCollateral:   v0.ClientCollateral,
	}, nil
}

func (s *state0) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn0)(nil)

func decodePublishStorageDealsReturn0(b []byte) (PublishStorageDealsReturn, error) {
	var retval market0.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn0{retval}, nil
}

type publishStorageDealsReturn0 struct {
	market0.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn0) IsDealValid(index uint64) (bool, int, error) {

	// PublishStorageDeals only succeeded if all deals were valid in this version of actors
	return true, int(index), nil

}

func (r *publishStorageDealsReturn0) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state0) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	return verifregtypes.NoAllocationID, xerrors.Errorf("unsupported before actors v9")

}

func (s *state0) ActorKey() string {
	return manifest.MarketKey
}

func (s *state0) ActorVersion() actorstypes.Version {
	return actorstypes.Version0
}

func (s *state0) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state0) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
