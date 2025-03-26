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
	market16 "github.com/filecoin-project/go-state-types/builtin/v16/market"
	adt16 "github.com/filecoin-project/go-state-types/builtin/v16/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state16)(nil)

func load16(store adt.Store, root cid.Cid) (State, error) {
	out := state16{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make16(store adt.Store) (State, error) {
	out := state16{store: store}

	s, err := market16.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state16 struct {
	market16.State
	store adt.Store
}

func (s *state16) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state16) BalancesChanged(otherState State) (bool, error) {
	otherState16, ok := otherState.(*state16)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState16.State.EscrowTable) || !s.State.LockedTable.Equals(otherState16.State.LockedTable), nil
}

func (s *state16) StatesChanged(otherState State) (bool, error) {
	otherState16, ok := otherState.(*state16)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState16.State.States), nil
}

func (s *state16) States() (DealStates, error) {
	stateArray, err := adt16.AsArray(s.store, s.State.States, market16.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates16{stateArray}, nil
}

func (s *state16) ProposalsChanged(otherState State) (bool, error) {
	otherState16, ok := otherState.(*state16)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState16.State.Proposals), nil
}

func (s *state16) Proposals() (DealProposals, error) {
	proposalArray, err := adt16.AsArray(s.store, s.State.Proposals, market16.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals16{proposalArray}, nil
}

func (s *state16) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt16.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals16{proposalCidSet}, nil
}

func (s *state16) EscrowTable() (BalanceTable, error) {
	bt, err := adt16.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable16{bt}, nil
}

func (s *state16) LockedTable() (BalanceTable, error) {
	bt, err := adt16.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable16{bt}, nil
}

func (s *state16) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market16.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state16) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable16 struct {
	*adt16.BalanceTable
}

func (bt *balanceTable16) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt16.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates16 struct {
	adt.Array
}

func (s *dealStates16) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal16 market16.DealState
	found, err := s.Array.Get(uint64(dealID), &deal16)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV16DealState(deal16)
	return deal, true, nil
}

func (s *dealStates16) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds16 market16.DealState
	return s.Array.ForEach(&ds16, func(idx int64) error {
		return cb(abi.DealID(idx), fromV16DealState(ds16))
	})
}

func (s *dealStates16) decode(val *cbg.Deferred) (DealState, error) {
	var ds16 market16.DealState
	if err := ds16.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV16DealState(ds16)
	return ds, nil
}

func (s *dealStates16) array() adt.Array {
	return s.Array
}

type dealStateV16 struct {
	ds16 market16.DealState
}

func (d dealStateV16) SectorNumber() abi.SectorNumber {

	return d.ds16.SectorNumber

}

func (d dealStateV16) SectorStartEpoch() abi.ChainEpoch {
	return d.ds16.SectorStartEpoch
}

func (d dealStateV16) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds16.LastUpdatedEpoch
}

func (d dealStateV16) SlashEpoch() abi.ChainEpoch {
	return d.ds16.SlashEpoch
}

func (d dealStateV16) Equals(other DealState) bool {
	if ov16, ok := other.(dealStateV16); ok {
		return d.ds16 == ov16.ds16
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

var _ DealState = (*dealStateV16)(nil)

func fromV16DealState(v16 market16.DealState) DealState {
	return dealStateV16{v16}
}

type dealProposals16 struct {
	adt.Array
}

func (s *dealProposals16) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal16 market16.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal16)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV16DealProposal(proposal16)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals16) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp16 market16.DealProposal
	return s.Array.ForEach(&dp16, func(idx int64) error {
		dp, err := fromV16DealProposal(dp16)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals16) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp16 market16.DealProposal
	if err := dp16.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV16DealProposal(dp16)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals16) array() adt.Array {
	return s.Array
}

type pendingProposals16 struct {
	*adt16.Set
}

func (s *pendingProposals16) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV16DealProposal(v16 market16.DealProposal) (DealProposal, error) {

	label, err := fromV16Label(v16.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v16.PieceCID,
		PieceSize:    v16.PieceSize,
		VerifiedDeal: v16.VerifiedDeal,
		Client:       v16.Client,
		Provider:     v16.Provider,

		Label: label,

		StartEpoch:           v16.StartEpoch,
		EndEpoch:             v16.EndEpoch,
		StoragePricePerEpoch: v16.StoragePricePerEpoch,

		ProviderCollateral: v16.ProviderCollateral,
		ClientCollateral:   v16.ClientCollateral,
	}, nil
}

func fromV16Label(v16 market16.DealLabel) (DealLabel, error) {
	if v16.IsString() {
		str, err := v16.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v16.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state16) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn16)(nil)

func decodePublishStorageDealsReturn16(b []byte) (PublishStorageDealsReturn, error) {
	var retval market16.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn16{retval}, nil
}

type publishStorageDealsReturn16 struct {
	market16.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn16) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn16) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state16) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt16.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state16) ActorKey() string {
	return manifest.MarketKey
}

func (s *state16) ActorVersion() actorstypes.Version {
	return actorstypes.Version16
}

func (s *state16) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state16) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt16.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors16{proverSectors, s.store}, nil

}

type providerSectors16 struct {
	*adt16.Map
	adt16.Store
}

type sectorDealIDs16 struct {
	*adt16.Map
}

func (s *providerSectors16) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt16.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs16{sectorDealIds}, true, nil
}

func (s *sectorDealIDs16) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs16) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
