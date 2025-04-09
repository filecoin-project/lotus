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
	market15 "github.com/filecoin-project/go-state-types/builtin/v15/market"
	adt15 "github.com/filecoin-project/go-state-types/builtin/v15/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state15)(nil)

func load15(store adt.Store, root cid.Cid) (State, error) {
	out := state15{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make15(store adt.Store) (State, error) {
	out := state15{store: store}

	s, err := market15.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state15 struct {
	market15.State
	store adt.Store
}

func (s *state15) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state15) BalancesChanged(otherState State) (bool, error) {
	otherState15, ok := otherState.(*state15)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState15.State.EscrowTable) || !s.State.LockedTable.Equals(otherState15.State.LockedTable), nil
}

func (s *state15) StatesChanged(otherState State) (bool, error) {
	otherState15, ok := otherState.(*state15)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState15.State.States), nil
}

func (s *state15) States() (DealStates, error) {
	stateArray, err := adt15.AsArray(s.store, s.State.States, market15.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates15{stateArray}, nil
}

func (s *state15) ProposalsChanged(otherState State) (bool, error) {
	otherState15, ok := otherState.(*state15)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState15.State.Proposals), nil
}

func (s *state15) Proposals() (DealProposals, error) {
	proposalArray, err := adt15.AsArray(s.store, s.State.Proposals, market15.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals15{proposalArray}, nil
}

func (s *state15) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt15.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals15{proposalCidSet}, nil
}

func (s *state15) EscrowTable() (BalanceTable, error) {
	bt, err := adt15.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable15{bt}, nil
}

func (s *state15) LockedTable() (BalanceTable, error) {
	bt, err := adt15.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable15{bt}, nil
}

func (s *state15) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market15.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state15) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable15 struct {
	*adt15.BalanceTable
}

func (bt *balanceTable15) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt15.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates15 struct {
	adt.Array
}

func (s *dealStates15) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal15 market15.DealState
	found, err := s.Array.Get(uint64(dealID), &deal15)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV15DealState(deal15)
	return deal, true, nil
}

func (s *dealStates15) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds15 market15.DealState
	return s.Array.ForEach(&ds15, func(idx int64) error {
		return cb(abi.DealID(idx), fromV15DealState(ds15))
	})
}

func (s *dealStates15) decode(val *cbg.Deferred) (DealState, error) {
	var ds15 market15.DealState
	if err := ds15.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV15DealState(ds15)
	return ds, nil
}

func (s *dealStates15) array() adt.Array {
	return s.Array
}

type dealStateV15 struct {
	ds15 market15.DealState
}

func (d dealStateV15) SectorNumber() abi.SectorNumber {

	return d.ds15.SectorNumber

}

func (d dealStateV15) SectorStartEpoch() abi.ChainEpoch {
	return d.ds15.SectorStartEpoch
}

func (d dealStateV15) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds15.LastUpdatedEpoch
}

func (d dealStateV15) SlashEpoch() abi.ChainEpoch {
	return d.ds15.SlashEpoch
}

func (d dealStateV15) Equals(other DealState) bool {
	if ov15, ok := other.(dealStateV15); ok {
		return d.ds15 == ov15.ds15
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

var _ DealState = (*dealStateV15)(nil)

func fromV15DealState(v15 market15.DealState) DealState {
	return dealStateV15{v15}
}

type dealProposals15 struct {
	adt.Array
}

func (s *dealProposals15) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal15 market15.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal15)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV15DealProposal(proposal15)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals15) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp15 market15.DealProposal
	return s.Array.ForEach(&dp15, func(idx int64) error {
		dp, err := fromV15DealProposal(dp15)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals15) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp15 market15.DealProposal
	if err := dp15.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV15DealProposal(dp15)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals15) array() adt.Array {
	return s.Array
}

type pendingProposals15 struct {
	*adt15.Set
}

func (s *pendingProposals15) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV15DealProposal(v15 market15.DealProposal) (DealProposal, error) {

	label, err := fromV15Label(v15.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v15.PieceCID,
		PieceSize:    v15.PieceSize,
		VerifiedDeal: v15.VerifiedDeal,
		Client:       v15.Client,
		Provider:     v15.Provider,

		Label: label,

		StartEpoch:           v15.StartEpoch,
		EndEpoch:             v15.EndEpoch,
		StoragePricePerEpoch: v15.StoragePricePerEpoch,

		ProviderCollateral: v15.ProviderCollateral,
		ClientCollateral:   v15.ClientCollateral,
	}, nil
}

func fromV15Label(v15 market15.DealLabel) (DealLabel, error) {
	if v15.IsString() {
		str, err := v15.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v15.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state15) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn15)(nil)

func decodePublishStorageDealsReturn15(b []byte) (PublishStorageDealsReturn, error) {
	var retval market15.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn15{retval}, nil
}

type publishStorageDealsReturn15 struct {
	market15.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn15) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn15) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state15) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt15.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state15) ActorKey() string {
	return manifest.MarketKey
}

func (s *state15) ActorVersion() actorstypes.Version {
	return actorstypes.Version15
}

func (s *state15) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state15) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt15.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors15{proverSectors, s.store}, nil

}

type providerSectors15 struct {
	*adt15.Map
	adt15.Store
}

type sectorDealIDs15 struct {
	*adt15.Map
}

func (s *providerSectors15) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt15.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs15{sectorDealIds}, true, nil
}

func (s *sectorDealIDs15) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs15) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
