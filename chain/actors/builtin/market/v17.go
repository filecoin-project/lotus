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
	market17 "github.com/filecoin-project/go-state-types/builtin/v17/market"
	adt17 "github.com/filecoin-project/go-state-types/builtin/v17/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state17)(nil)

func load17(store adt.Store, root cid.Cid) (State, error) {
	out := state17{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make17(store adt.Store) (State, error) {
	out := state17{store: store}

	s, err := market17.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state17 struct {
	market17.State
	store adt.Store
}

func (s *state17) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state17) BalancesChanged(otherState State) (bool, error) {
	otherState17, ok := otherState.(*state17)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState17.State.EscrowTable) || !s.State.LockedTable.Equals(otherState17.State.LockedTable), nil
}

func (s *state17) StatesChanged(otherState State) (bool, error) {
	otherState17, ok := otherState.(*state17)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState17.State.States), nil
}

func (s *state17) States() (DealStates, error) {
	stateArray, err := adt17.AsArray(s.store, s.State.States, market17.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates17{stateArray}, nil
}

func (s *state17) ProposalsChanged(otherState State) (bool, error) {
	otherState17, ok := otherState.(*state17)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState17.State.Proposals), nil
}

func (s *state17) Proposals() (DealProposals, error) {
	proposalArray, err := adt17.AsArray(s.store, s.State.Proposals, market17.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals17{proposalArray}, nil
}

func (s *state17) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt17.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals17{proposalCidSet}, nil
}

func (s *state17) EscrowTable() (BalanceTable, error) {
	bt, err := adt17.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable17{bt}, nil
}

func (s *state17) LockedTable() (BalanceTable, error) {
	bt, err := adt17.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable17{bt}, nil
}

func (s *state17) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market17.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state17) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable17 struct {
	*adt17.BalanceTable
}

func (bt *balanceTable17) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt17.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates17 struct {
	adt.Array
}

func (s *dealStates17) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal17 market17.DealState
	found, err := s.Array.Get(uint64(dealID), &deal17)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV17DealState(deal17)
	return deal, true, nil
}

func (s *dealStates17) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds17 market17.DealState
	return s.Array.ForEach(&ds17, func(idx int64) error {
		return cb(abi.DealID(idx), fromV17DealState(ds17))
	})
}

func (s *dealStates17) decode(val *cbg.Deferred) (DealState, error) {
	var ds17 market17.DealState
	if err := ds17.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV17DealState(ds17)
	return ds, nil
}

func (s *dealStates17) array() adt.Array {
	return s.Array
}

type dealStateV17 struct {
	ds17 market17.DealState
}

func (d dealStateV17) SectorNumber() abi.SectorNumber {

	return d.ds17.SectorNumber

}

func (d dealStateV17) SectorStartEpoch() abi.ChainEpoch {
	return d.ds17.SectorStartEpoch
}

func (d dealStateV17) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds17.LastUpdatedEpoch
}

func (d dealStateV17) SlashEpoch() abi.ChainEpoch {
	return d.ds17.SlashEpoch
}

func (d dealStateV17) Equals(other DealState) bool {
	if ov17, ok := other.(dealStateV17); ok {
		return d.ds17 == ov17.ds17
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

var _ DealState = (*dealStateV17)(nil)

func fromV17DealState(v17 market17.DealState) DealState {
	return dealStateV17{v17}
}

type dealProposals17 struct {
	adt.Array
}

func (s *dealProposals17) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal17 market17.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal17)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV17DealProposal(proposal17)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals17) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp17 market17.DealProposal
	return s.Array.ForEach(&dp17, func(idx int64) error {
		dp, err := fromV17DealProposal(dp17)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals17) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp17 market17.DealProposal
	if err := dp17.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV17DealProposal(dp17)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals17) array() adt.Array {
	return s.Array
}

type pendingProposals17 struct {
	*adt17.Set
}

func (s *pendingProposals17) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV17DealProposal(v17 market17.DealProposal) (DealProposal, error) {

	label, err := fromV17Label(v17.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v17.PieceCID,
		PieceSize:    v17.PieceSize,
		VerifiedDeal: v17.VerifiedDeal,
		Client:       v17.Client,
		Provider:     v17.Provider,

		Label: label,

		StartEpoch:           v17.StartEpoch,
		EndEpoch:             v17.EndEpoch,
		StoragePricePerEpoch: v17.StoragePricePerEpoch,

		ProviderCollateral: v17.ProviderCollateral,
		ClientCollateral:   v17.ClientCollateral,
	}, nil
}

func fromV17Label(v17 market17.DealLabel) (DealLabel, error) {
	if v17.IsString() {
		str, err := v17.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v17.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state17) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn17)(nil)

func decodePublishStorageDealsReturn17(b []byte) (PublishStorageDealsReturn, error) {
	var retval market17.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn17{retval}, nil
}

type publishStorageDealsReturn17 struct {
	market17.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn17) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn17) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state17) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt17.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state17) ActorKey() string {
	return manifest.MarketKey
}

func (s *state17) ActorVersion() actorstypes.Version {
	return actorstypes.Version17
}

func (s *state17) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state17) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt17.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors17{proverSectors, s.store}, nil

}

type providerSectors17 struct {
	*adt17.Map
	adt17.Store
}

type sectorDealIDs17 struct {
	*adt17.Map
}

func (s *providerSectors17) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt17.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs17{sectorDealIds}, true, nil
}

func (s *sectorDealIDs17) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs17) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
	var dealIds abi.DealIDList
	found, err := s.Map.Get(abi.UIntKey(uint64(sectorNumber)), &dealIds)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for sector %d: %w", sectorNumber, err)
	}
	if !found {
		return nil, false, nil
	}
	return dealIds, true, nil
}
