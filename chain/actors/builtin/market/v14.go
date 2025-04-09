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
	market14 "github.com/filecoin-project/go-state-types/builtin/v14/market"
	adt14 "github.com/filecoin-project/go-state-types/builtin/v14/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store) (State, error) {
	out := state14{store: store}

	s, err := market14.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state14 struct {
	market14.State
	store adt.Store
}

func (s *state14) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state14) BalancesChanged(otherState State) (bool, error) {
	otherState14, ok := otherState.(*state14)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState14.State.EscrowTable) || !s.State.LockedTable.Equals(otherState14.State.LockedTable), nil
}

func (s *state14) StatesChanged(otherState State) (bool, error) {
	otherState14, ok := otherState.(*state14)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState14.State.States), nil
}

func (s *state14) States() (DealStates, error) {
	stateArray, err := adt14.AsArray(s.store, s.State.States, market14.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates14{stateArray}, nil
}

func (s *state14) ProposalsChanged(otherState State) (bool, error) {
	otherState14, ok := otherState.(*state14)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState14.State.Proposals), nil
}

func (s *state14) Proposals() (DealProposals, error) {
	proposalArray, err := adt14.AsArray(s.store, s.State.Proposals, market14.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals14{proposalArray}, nil
}

func (s *state14) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt14.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals14{proposalCidSet}, nil
}

func (s *state14) EscrowTable() (BalanceTable, error) {
	bt, err := adt14.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable14{bt}, nil
}

func (s *state14) LockedTable() (BalanceTable, error) {
	bt, err := adt14.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable14{bt}, nil
}

func (s *state14) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market14.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state14) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable14 struct {
	*adt14.BalanceTable
}

func (bt *balanceTable14) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt14.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates14 struct {
	adt.Array
}

func (s *dealStates14) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal14 market14.DealState
	found, err := s.Array.Get(uint64(dealID), &deal14)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV14DealState(deal14)
	return deal, true, nil
}

func (s *dealStates14) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds14 market14.DealState
	return s.Array.ForEach(&ds14, func(idx int64) error {
		return cb(abi.DealID(idx), fromV14DealState(ds14))
	})
}

func (s *dealStates14) decode(val *cbg.Deferred) (DealState, error) {
	var ds14 market14.DealState
	if err := ds14.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV14DealState(ds14)
	return ds, nil
}

func (s *dealStates14) array() adt.Array {
	return s.Array
}

type dealStateV14 struct {
	ds14 market14.DealState
}

func (d dealStateV14) SectorNumber() abi.SectorNumber {

	return d.ds14.SectorNumber

}

func (d dealStateV14) SectorStartEpoch() abi.ChainEpoch {
	return d.ds14.SectorStartEpoch
}

func (d dealStateV14) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds14.LastUpdatedEpoch
}

func (d dealStateV14) SlashEpoch() abi.ChainEpoch {
	return d.ds14.SlashEpoch
}

func (d dealStateV14) Equals(other DealState) bool {
	if ov14, ok := other.(dealStateV14); ok {
		return d.ds14 == ov14.ds14
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

var _ DealState = (*dealStateV14)(nil)

func fromV14DealState(v14 market14.DealState) DealState {
	return dealStateV14{v14}
}

type dealProposals14 struct {
	adt.Array
}

func (s *dealProposals14) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal14 market14.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal14)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV14DealProposal(proposal14)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals14) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp14 market14.DealProposal
	return s.Array.ForEach(&dp14, func(idx int64) error {
		dp, err := fromV14DealProposal(dp14)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals14) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp14 market14.DealProposal
	if err := dp14.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV14DealProposal(dp14)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals14) array() adt.Array {
	return s.Array
}

type pendingProposals14 struct {
	*adt14.Set
}

func (s *pendingProposals14) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV14DealProposal(v14 market14.DealProposal) (DealProposal, error) {

	label, err := fromV14Label(v14.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v14.PieceCID,
		PieceSize:    v14.PieceSize,
		VerifiedDeal: v14.VerifiedDeal,
		Client:       v14.Client,
		Provider:     v14.Provider,

		Label: label,

		StartEpoch:           v14.StartEpoch,
		EndEpoch:             v14.EndEpoch,
		StoragePricePerEpoch: v14.StoragePricePerEpoch,

		ProviderCollateral: v14.ProviderCollateral,
		ClientCollateral:   v14.ClientCollateral,
	}, nil
}

func fromV14Label(v14 market14.DealLabel) (DealLabel, error) {
	if v14.IsString() {
		str, err := v14.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v14.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state14) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn14)(nil)

func decodePublishStorageDealsReturn14(b []byte) (PublishStorageDealsReturn, error) {
	var retval market14.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn14{retval}, nil
}

type publishStorageDealsReturn14 struct {
	market14.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn14) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn14) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state14) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt14.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state14) ActorKey() string {
	return manifest.MarketKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state14) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt14.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors14{proverSectors, s.store}, nil

}

type providerSectors14 struct {
	*adt14.Map
	adt14.Store
}

type sectorDealIDs14 struct {
	*adt14.Map
}

func (s *providerSectors14) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt14.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs14{sectorDealIds}, true, nil
}

func (s *sectorDealIDs14) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs14) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
