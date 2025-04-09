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
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store) (State, error) {
	out := state9{store: store}

	s, err := market9.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state9 struct {
	market9.State
	store adt.Store
}

func (s *state9) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state9) BalancesChanged(otherState State) (bool, error) {
	otherState9, ok := otherState.(*state9)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState9.State.EscrowTable) || !s.State.LockedTable.Equals(otherState9.State.LockedTable), nil
}

func (s *state9) StatesChanged(otherState State) (bool, error) {
	otherState9, ok := otherState.(*state9)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState9.State.States), nil
}

func (s *state9) States() (DealStates, error) {
	stateArray, err := adt9.AsArray(s.store, s.State.States, market9.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates9{stateArray}, nil
}

func (s *state9) ProposalsChanged(otherState State) (bool, error) {
	otherState9, ok := otherState.(*state9)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState9.State.Proposals), nil
}

func (s *state9) Proposals() (DealProposals, error) {
	proposalArray, err := adt9.AsArray(s.store, s.State.Proposals, market9.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals9{proposalArray}, nil
}

func (s *state9) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt9.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals9{proposalCidSet}, nil
}

func (s *state9) EscrowTable() (BalanceTable, error) {
	bt, err := adt9.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable9{bt}, nil
}

func (s *state9) LockedTable() (BalanceTable, error) {
	bt, err := adt9.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable9{bt}, nil
}

func (s *state9) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market9.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state9) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable9 struct {
	*adt9.BalanceTable
}

func (bt *balanceTable9) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt9.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates9 struct {
	adt.Array
}

func (s *dealStates9) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal9 market9.DealState
	found, err := s.Array.Get(uint64(dealID), &deal9)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV9DealState(deal9)
	return deal, true, nil
}

func (s *dealStates9) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds9 market9.DealState
	return s.Array.ForEach(&ds9, func(idx int64) error {
		return cb(abi.DealID(idx), fromV9DealState(ds9))
	})
}

func (s *dealStates9) decode(val *cbg.Deferred) (DealState, error) {
	var ds9 market9.DealState
	if err := ds9.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV9DealState(ds9)
	return ds, nil
}

func (s *dealStates9) array() adt.Array {
	return s.Array
}

type dealStateV9 struct {
	ds9 market9.DealState
}

func (d dealStateV9) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV9) SectorStartEpoch() abi.ChainEpoch {
	return d.ds9.SectorStartEpoch
}

func (d dealStateV9) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds9.LastUpdatedEpoch
}

func (d dealStateV9) SlashEpoch() abi.ChainEpoch {
	return d.ds9.SlashEpoch
}

func (d dealStateV9) Equals(other DealState) bool {
	if ov9, ok := other.(dealStateV9); ok {
		return d.ds9 == ov9.ds9
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

var _ DealState = (*dealStateV9)(nil)

func fromV9DealState(v9 market9.DealState) DealState {
	return dealStateV9{v9}
}

type dealProposals9 struct {
	adt.Array
}

func (s *dealProposals9) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal9 market9.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal9)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV9DealProposal(proposal9)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals9) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp9 market9.DealProposal
	return s.Array.ForEach(&dp9, func(idx int64) error {
		dp, err := fromV9DealProposal(dp9)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals9) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp9 market9.DealProposal
	if err := dp9.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV9DealProposal(dp9)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals9) array() adt.Array {
	return s.Array
}

type pendingProposals9 struct {
	*adt9.Set
}

func (s *pendingProposals9) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV9DealProposal(v9 market9.DealProposal) (DealProposal, error) {

	label, err := fromV9Label(v9.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v9.PieceCID,
		PieceSize:    v9.PieceSize,
		VerifiedDeal: v9.VerifiedDeal,
		Client:       v9.Client,
		Provider:     v9.Provider,

		Label: label,

		StartEpoch:           v9.StartEpoch,
		EndEpoch:             v9.EndEpoch,
		StoragePricePerEpoch: v9.StoragePricePerEpoch,

		ProviderCollateral: v9.ProviderCollateral,
		ClientCollateral:   v9.ClientCollateral,
	}, nil
}

func fromV9Label(v9 market9.DealLabel) (DealLabel, error) {
	if v9.IsString() {
		str, err := v9.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v9.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state9) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn9)(nil)

func decodePublishStorageDealsReturn9(b []byte) (PublishStorageDealsReturn, error) {
	var retval market9.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn9{retval}, nil
}

type publishStorageDealsReturn9 struct {
	market9.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn9) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn9) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state9) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt9.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state9) ActorKey() string {
	return manifest.MarketKey
}

func (s *state9) ActorVersion() actorstypes.Version {
	return actorstypes.Version9
}

func (s *state9) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state9) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
