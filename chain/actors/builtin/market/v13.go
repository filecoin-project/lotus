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
	market13 "github.com/filecoin-project/go-state-types/builtin/v13/market"
	adt13 "github.com/filecoin-project/go-state-types/builtin/v13/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state13)(nil)

func load13(store adt.Store, root cid.Cid) (State, error) {
	out := state13{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make13(store adt.Store) (State, error) {
	out := state13{store: store}

	s, err := market13.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state13 struct {
	market13.State
	store adt.Store
}

func (s *state13) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state13) BalancesChanged(otherState State) (bool, error) {
	otherState13, ok := otherState.(*state13)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState13.State.EscrowTable) || !s.State.LockedTable.Equals(otherState13.State.LockedTable), nil
}

func (s *state13) StatesChanged(otherState State) (bool, error) {
	otherState13, ok := otherState.(*state13)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState13.State.States), nil
}

func (s *state13) States() (DealStates, error) {
	stateArray, err := adt13.AsArray(s.store, s.State.States, market13.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates13{stateArray}, nil
}

func (s *state13) ProposalsChanged(otherState State) (bool, error) {
	otherState13, ok := otherState.(*state13)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState13.State.Proposals), nil
}

func (s *state13) Proposals() (DealProposals, error) {
	proposalArray, err := adt13.AsArray(s.store, s.State.Proposals, market13.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals13{proposalArray}, nil
}

func (s *state13) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt13.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals13{proposalCidSet}, nil
}

func (s *state13) EscrowTable() (BalanceTable, error) {
	bt, err := adt13.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable13{bt}, nil
}

func (s *state13) LockedTable() (BalanceTable, error) {
	bt, err := adt13.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable13{bt}, nil
}

func (s *state13) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market13.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state13) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable13 struct {
	*adt13.BalanceTable
}

func (bt *balanceTable13) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt13.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates13 struct {
	adt.Array
}

func (s *dealStates13) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal13 market13.DealState
	found, err := s.Array.Get(uint64(dealID), &deal13)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV13DealState(deal13)
	return deal, true, nil
}

func (s *dealStates13) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds13 market13.DealState
	return s.Array.ForEach(&ds13, func(idx int64) error {
		return cb(abi.DealID(idx), fromV13DealState(ds13))
	})
}

func (s *dealStates13) decode(val *cbg.Deferred) (DealState, error) {
	var ds13 market13.DealState
	if err := ds13.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV13DealState(ds13)
	return ds, nil
}

func (s *dealStates13) array() adt.Array {
	return s.Array
}

type dealStateV13 struct {
	ds13 market13.DealState
}

func (d dealStateV13) SectorNumber() abi.SectorNumber {

	return d.ds13.SectorNumber

}

func (d dealStateV13) SectorStartEpoch() abi.ChainEpoch {
	return d.ds13.SectorStartEpoch
}

func (d dealStateV13) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds13.LastUpdatedEpoch
}

func (d dealStateV13) SlashEpoch() abi.ChainEpoch {
	return d.ds13.SlashEpoch
}

func (d dealStateV13) Equals(other DealState) bool {
	if ov13, ok := other.(dealStateV13); ok {
		return d.ds13 == ov13.ds13
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

var _ DealState = (*dealStateV13)(nil)

func fromV13DealState(v13 market13.DealState) DealState {
	return dealStateV13{v13}
}

type dealProposals13 struct {
	adt.Array
}

func (s *dealProposals13) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal13 market13.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal13)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV13DealProposal(proposal13)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals13) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp13 market13.DealProposal
	return s.Array.ForEach(&dp13, func(idx int64) error {
		dp, err := fromV13DealProposal(dp13)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals13) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp13 market13.DealProposal
	if err := dp13.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV13DealProposal(dp13)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals13) array() adt.Array {
	return s.Array
}

type pendingProposals13 struct {
	*adt13.Set
}

func (s *pendingProposals13) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV13DealProposal(v13 market13.DealProposal) (DealProposal, error) {

	label, err := fromV13Label(v13.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v13.PieceCID,
		PieceSize:    v13.PieceSize,
		VerifiedDeal: v13.VerifiedDeal,
		Client:       v13.Client,
		Provider:     v13.Provider,

		Label: label,

		StartEpoch:           v13.StartEpoch,
		EndEpoch:             v13.EndEpoch,
		StoragePricePerEpoch: v13.StoragePricePerEpoch,

		ProviderCollateral: v13.ProviderCollateral,
		ClientCollateral:   v13.ClientCollateral,
	}, nil
}

func fromV13Label(v13 market13.DealLabel) (DealLabel, error) {
	if v13.IsString() {
		str, err := v13.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v13.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state13) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn13)(nil)

func decodePublishStorageDealsReturn13(b []byte) (PublishStorageDealsReturn, error) {
	var retval market13.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn13{retval}, nil
}

type publishStorageDealsReturn13 struct {
	market13.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn13) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn13) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state13) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt13.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state13) ActorKey() string {
	return manifest.MarketKey
}

func (s *state13) ActorVersion() actorstypes.Version {
	return actorstypes.Version13
}

func (s *state13) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state13) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt13.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors13{proverSectors, s.store}, nil

}

type providerSectors13 struct {
	*adt13.Map
	adt13.Store
}

type sectorDealIDs13 struct {
	*adt13.Map
}

func (s *providerSectors13) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt13.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs13{sectorDealIds}, true, nil
}

func (s *sectorDealIDs13) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs13) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
