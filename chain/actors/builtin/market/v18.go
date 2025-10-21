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
	market18 "github.com/filecoin-project/go-state-types/builtin/v18/market"
	adt18 "github.com/filecoin-project/go-state-types/builtin/v18/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state18)(nil)

func load18(store adt.Store, root cid.Cid) (State, error) {
	out := state18{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make18(store adt.Store) (State, error) {
	out := state18{store: store}

	s, err := market18.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state18 struct {
	market18.State
	store adt.Store
}

func (s *state18) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state18) BalancesChanged(otherState State) (bool, error) {
	otherState18, ok := otherState.(*state18)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState18.State.EscrowTable) || !s.State.LockedTable.Equals(otherState18.State.LockedTable), nil
}

func (s *state18) StatesChanged(otherState State) (bool, error) {
	otherState18, ok := otherState.(*state18)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState18.State.States), nil
}

func (s *state18) States() (DealStates, error) {
	stateArray, err := adt18.AsArray(s.store, s.State.States, market18.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates18{stateArray}, nil
}

func (s *state18) ProposalsChanged(otherState State) (bool, error) {
	otherState18, ok := otherState.(*state18)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState18.State.Proposals), nil
}

func (s *state18) Proposals() (DealProposals, error) {
	proposalArray, err := adt18.AsArray(s.store, s.State.Proposals, market18.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals18{proposalArray}, nil
}

func (s *state18) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt18.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals18{proposalCidSet}, nil
}

func (s *state18) EscrowTable() (BalanceTable, error) {
	bt, err := adt18.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable18{bt}, nil
}

func (s *state18) LockedTable() (BalanceTable, error) {
	bt, err := adt18.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable18{bt}, nil
}

func (s *state18) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market18.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state18) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable18 struct {
	*adt18.BalanceTable
}

func (bt *balanceTable18) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt18.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates18 struct {
	adt.Array
}

func (s *dealStates18) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal18 market18.DealState
	found, err := s.Array.Get(uint64(dealID), &deal18)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV18DealState(deal18)
	return deal, true, nil
}

func (s *dealStates18) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds18 market18.DealState
	return s.Array.ForEach(&ds18, func(idx int64) error {
		return cb(abi.DealID(idx), fromV18DealState(ds18))
	})
}

func (s *dealStates18) decode(val *cbg.Deferred) (DealState, error) {
	var ds18 market18.DealState
	if err := ds18.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV18DealState(ds18)
	return ds, nil
}

func (s *dealStates18) array() adt.Array {
	return s.Array
}

type dealStateV18 struct {
	ds18 market18.DealState
}

func (d dealStateV18) SectorNumber() abi.SectorNumber {

	return d.ds18.SectorNumber

}

func (d dealStateV18) SectorStartEpoch() abi.ChainEpoch {
	return d.ds18.SectorStartEpoch
}

func (d dealStateV18) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds18.LastUpdatedEpoch
}

func (d dealStateV18) SlashEpoch() abi.ChainEpoch {
	return d.ds18.SlashEpoch
}

func (d dealStateV18) Equals(other DealState) bool {
	if ov18, ok := other.(dealStateV18); ok {
		return d.ds18 == ov18.ds18
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

var _ DealState = (*dealStateV18)(nil)

func fromV18DealState(v18 market18.DealState) DealState {
	return dealStateV18{v18}
}

type dealProposals18 struct {
	adt.Array
}

func (s *dealProposals18) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal18 market18.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal18)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV18DealProposal(proposal18)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals18) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp18 market18.DealProposal
	return s.Array.ForEach(&dp18, func(idx int64) error {
		dp, err := fromV18DealProposal(dp18)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals18) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp18 market18.DealProposal
	if err := dp18.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV18DealProposal(dp18)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals18) array() adt.Array {
	return s.Array
}

type pendingProposals18 struct {
	*adt18.Set
}

func (s *pendingProposals18) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV18DealProposal(v18 market18.DealProposal) (DealProposal, error) {

	label, err := fromV18Label(v18.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v18.PieceCID,
		PieceSize:    v18.PieceSize,
		VerifiedDeal: v18.VerifiedDeal,
		Client:       v18.Client,
		Provider:     v18.Provider,

		Label: label,

		StartEpoch:           v18.StartEpoch,
		EndEpoch:             v18.EndEpoch,
		StoragePricePerEpoch: v18.StoragePricePerEpoch,

		ProviderCollateral: v18.ProviderCollateral,
		ClientCollateral:   v18.ClientCollateral,
	}, nil
}

func fromV18Label(v18 market18.DealLabel) (DealLabel, error) {
	if v18.IsString() {
		str, err := v18.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v18.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state18) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn18)(nil)

func decodePublishStorageDealsReturn18(b []byte) (PublishStorageDealsReturn, error) {
	var retval market18.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn18{retval}, nil
}

type publishStorageDealsReturn18 struct {
	market18.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn18) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn18) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state18) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt18.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state18) ActorKey() string {
	return manifest.MarketKey
}

func (s *state18) ActorVersion() actorstypes.Version {
	return actorstypes.Version18
}

func (s *state18) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state18) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt18.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors18{proverSectors, s.store}, nil

}

type providerSectors18 struct {
	*adt18.Map
	adt18.Store
}

type sectorDealIDs18 struct {
	*adt18.Map
}

func (s *providerSectors18) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt18.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs18{sectorDealIds}, true, nil
}

func (s *sectorDealIDs18) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs18) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
