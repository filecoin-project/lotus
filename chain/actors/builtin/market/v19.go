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
	market19 "github.com/filecoin-project/go-state-types/builtin/v19/market"
	adt19 "github.com/filecoin-project/go-state-types/builtin/v19/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state19)(nil)

func load19(store adt.Store, root cid.Cid) (State, error) {
	out := state19{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make19(store adt.Store) (State, error) {
	out := state19{store: store}

	s, err := market19.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state19 struct {
	market19.State
	store adt.Store
}

func (s *state19) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state19) BalancesChanged(otherState State) (bool, error) {
	otherState19, ok := otherState.(*state19)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState19.State.EscrowTable) || !s.State.LockedTable.Equals(otherState19.State.LockedTable), nil
}

func (s *state19) StatesChanged(otherState State) (bool, error) {
	otherState19, ok := otherState.(*state19)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState19.State.States), nil
}

func (s *state19) States() (DealStates, error) {
	stateArray, err := adt19.AsArray(s.store, s.State.States, market19.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates19{stateArray}, nil
}

func (s *state19) ProposalsChanged(otherState State) (bool, error) {
	otherState19, ok := otherState.(*state19)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState19.State.Proposals), nil
}

func (s *state19) Proposals() (DealProposals, error) {
	proposalArray, err := adt19.AsArray(s.store, s.State.Proposals, market19.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals19{proposalArray}, nil
}

func (s *state19) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt19.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals19{proposalCidSet}, nil
}

func (s *state19) EscrowTable() (BalanceTable, error) {
	bt, err := adt19.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable19{bt}, nil
}

func (s *state19) LockedTable() (BalanceTable, error) {
	bt, err := adt19.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable19{bt}, nil
}

func (s *state19) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market19.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state19) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable19 struct {
	*adt19.BalanceTable
}

func (bt *balanceTable19) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt19.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates19 struct {
	adt.Array
}

func (s *dealStates19) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal19 market19.DealState
	found, err := s.Array.Get(uint64(dealID), &deal19)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV19DealState(deal19)
	return deal, true, nil
}

func (s *dealStates19) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds19 market19.DealState
	return s.Array.ForEach(&ds19, func(idx int64) error {
		return cb(abi.DealID(idx), fromV19DealState(ds19))
	})
}

func (s *dealStates19) decode(val *cbg.Deferred) (DealState, error) {
	var ds19 market19.DealState
	if err := ds19.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV19DealState(ds19)
	return ds, nil
}

func (s *dealStates19) array() adt.Array {
	return s.Array
}

type dealStateV19 struct {
	ds19 market19.DealState
}

func (d dealStateV19) SectorNumber() abi.SectorNumber {

	return d.ds19.SectorNumber

}

func (d dealStateV19) SectorStartEpoch() abi.ChainEpoch {
	return d.ds19.SectorStartEpoch
}

func (d dealStateV19) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds19.LastUpdatedEpoch
}

func (d dealStateV19) SlashEpoch() abi.ChainEpoch {
	return d.ds19.SlashEpoch
}

func (d dealStateV19) Equals(other DealState) bool {
	if ov19, ok := other.(dealStateV19); ok {
		return d.ds19 == ov19.ds19
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

var _ DealState = (*dealStateV19)(nil)

func fromV19DealState(v19 market19.DealState) DealState {
	return dealStateV19{v19}
}

type dealProposals19 struct {
	adt.Array
}

func (s *dealProposals19) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal19 market19.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal19)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV19DealProposal(proposal19)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals19) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp19 market19.DealProposal
	return s.Array.ForEach(&dp19, func(idx int64) error {
		dp, err := fromV19DealProposal(dp19)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals19) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp19 market19.DealProposal
	if err := dp19.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV19DealProposal(dp19)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals19) array() adt.Array {
	return s.Array
}

type pendingProposals19 struct {
	*adt19.Set
}

func (s *pendingProposals19) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV19DealProposal(v19 market19.DealProposal) (DealProposal, error) {

	label, err := fromV19Label(v19.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v19.PieceCID,
		PieceSize:    v19.PieceSize,
		VerifiedDeal: v19.VerifiedDeal,
		Client:       v19.Client,
		Provider:     v19.Provider,

		Label: label,

		StartEpoch:           v19.StartEpoch,
		EndEpoch:             v19.EndEpoch,
		StoragePricePerEpoch: v19.StoragePricePerEpoch,

		ProviderCollateral: v19.ProviderCollateral,
		ClientCollateral:   v19.ClientCollateral,
	}, nil
}

func fromV19Label(v19 market19.DealLabel) (DealLabel, error) {
	if v19.IsString() {
		str, err := v19.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v19.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state19) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn19)(nil)

func decodePublishStorageDealsReturn19(b []byte) (PublishStorageDealsReturn, error) {
	var retval market19.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn19{retval}, nil
}

type publishStorageDealsReturn19 struct {
	market19.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn19) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn19) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state19) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt19.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state19) ActorKey() string {
	return manifest.MarketKey
}

func (s *state19) ActorVersion() actorstypes.Version {
	return actorstypes.Version19
}

func (s *state19) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state19) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt19.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors19{proverSectors, s.store}, nil

}

type providerSectors19 struct {
	*adt19.Map
	adt19.Store
}

type sectorDealIDs19 struct {
	*adt19.Map
}

func (s *providerSectors19) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt19.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs19{sectorDealIds}, true, nil
}

func (s *sectorDealIDs19) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs19) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
