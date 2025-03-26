package market

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	market10 "github.com/filecoin-project/go-state-types/builtin/v10/market"
	adt10 "github.com/filecoin-project/go-state-types/builtin/v10/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store) (State, error) {
	out := state10{store: store}

	s, err := market10.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state10 struct {
	market10.State
	store adt.Store
}

func (s *state10) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state10) BalancesChanged(otherState State) (bool, error) {
	otherState10, ok := otherState.(*state10)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState10.State.EscrowTable) || !s.State.LockedTable.Equals(otherState10.State.LockedTable), nil
}

func (s *state10) StatesChanged(otherState State) (bool, error) {
	otherState10, ok := otherState.(*state10)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState10.State.States), nil
}

func (s *state10) States() (DealStates, error) {
	stateArray, err := adt10.AsArray(s.store, s.State.States, market10.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates10{stateArray}, nil
}

func (s *state10) ProposalsChanged(otherState State) (bool, error) {
	otherState10, ok := otherState.(*state10)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState10.State.Proposals), nil
}

func (s *state10) Proposals() (DealProposals, error) {
	proposalArray, err := adt10.AsArray(s.store, s.State.Proposals, market10.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals10{proposalArray}, nil
}

func (s *state10) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt10.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals10{proposalCidSet}, nil
}

func (s *state10) EscrowTable() (BalanceTable, error) {
	bt, err := adt10.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable10{bt}, nil
}

func (s *state10) LockedTable() (BalanceTable, error) {
	bt, err := adt10.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable10{bt}, nil
}

func (s *state10) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market10.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state10) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable10 struct {
	*adt10.BalanceTable
}

func (bt *balanceTable10) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt10.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates10 struct {
	adt.Array
}

func (s *dealStates10) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal10 market10.DealState
	found, err := s.Array.Get(uint64(dealID), &deal10)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV10DealState(deal10)
	return deal, true, nil
}

func (s *dealStates10) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds10 market10.DealState
	return s.Array.ForEach(&ds10, func(idx int64) error {
		return cb(abi.DealID(idx), fromV10DealState(ds10))
	})
}

func (s *dealStates10) decode(val *cbg.Deferred) (DealState, error) {
	var ds10 market10.DealState
	if err := ds10.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV10DealState(ds10)
	return ds, nil
}

func (s *dealStates10) array() adt.Array {
	return s.Array
}

type dealStateV10 struct {
	ds10 market10.DealState
}

func (d dealStateV10) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV10) SectorStartEpoch() abi.ChainEpoch {
	return d.ds10.SectorStartEpoch
}

func (d dealStateV10) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds10.LastUpdatedEpoch
}

func (d dealStateV10) SlashEpoch() abi.ChainEpoch {
	return d.ds10.SlashEpoch
}

func (d dealStateV10) Equals(other DealState) bool {
	if ov10, ok := other.(dealStateV10); ok {
		return d.ds10 == ov10.ds10
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

var _ DealState = (*dealStateV10)(nil)

func fromV10DealState(v10 market10.DealState) DealState {
	return dealStateV10{v10}
}

type dealProposals10 struct {
	adt.Array
}

func (s *dealProposals10) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal10 market10.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal10)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV10DealProposal(proposal10)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals10) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp10 market10.DealProposal
	return s.Array.ForEach(&dp10, func(idx int64) error {
		dp, err := fromV10DealProposal(dp10)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals10) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp10 market10.DealProposal
	if err := dp10.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV10DealProposal(dp10)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals10) array() adt.Array {
	return s.Array
}

type pendingProposals10 struct {
	*adt10.Set
}

func (s *pendingProposals10) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV10DealProposal(v10 market10.DealProposal) (DealProposal, error) {

	label, err := fromV10Label(v10.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v10.PieceCID,
		PieceSize:    v10.PieceSize,
		VerifiedDeal: v10.VerifiedDeal,
		Client:       v10.Client,
		Provider:     v10.Provider,

		Label: label,

		StartEpoch:           v10.StartEpoch,
		EndEpoch:             v10.EndEpoch,
		StoragePricePerEpoch: v10.StoragePricePerEpoch,

		ProviderCollateral: v10.ProviderCollateral,
		ClientCollateral:   v10.ClientCollateral,
	}, nil
}

func fromV10Label(v10 market10.DealLabel) (DealLabel, error) {
	if v10.IsString() {
		str, err := v10.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v10.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state10) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn10)(nil)

func decodePublishStorageDealsReturn10(b []byte) (PublishStorageDealsReturn, error) {
	var retval market10.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn10{retval}, nil
}

type publishStorageDealsReturn10 struct {
	market10.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn10) IsDealValid(index uint64) (bool, int, error) {

	set, err := r.ValidDeals.IsSet(index)
	if err != nil || !set {
		return false, -1, err
	}
	maskBf, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{
		Runs: []rlepluslazy.Run{rlepluslazy.Run{Val: true, Len: index}}})
	if err != nil {
		return false, -1, err
	}
	before, err := bitfield.IntersectBitField(maskBf, r.ValidDeals)
	if err != nil {
		return false, -1, err
	}
	outIdx, err := before.Count()
	if err != nil {
		return false, -1, err
	}
	return set, int(outIdx), nil

}

func (r *publishStorageDealsReturn10) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state10) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt10.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
	if err != nil {
		return verifregtypes.NoAllocationID, xerrors.Errorf("failed to load allocation id for %d: %w", dealId, err)
	}

	var allocationId cbg.CborInt
	found, err := allocations.Get(abi.UIntKey(uint64(dealId)), &allocationId)
	if err != nil {
		return verifregtypes.NoAllocationID, xerrors.Errorf("failed to load allocation id for %d: %w", dealId, err)
	}
	if !found {
		return verifregtypes.NoAllocationID, nil
	}

	return verifregtypes.AllocationId(allocationId), nil

}

func (s *state10) ActorKey() string {
	return manifest.MarketKey
}

func (s *state10) ActorVersion() actorstypes.Version {
	return actorstypes.Version10
}

func (s *state10) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state10) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
