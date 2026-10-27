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
	market20 "github.com/filecoin-project/go-state-types/builtin/v20/market"
	adt20 "github.com/filecoin-project/go-state-types/builtin/v20/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state20)(nil)

func load20(store adt.Store, root cid.Cid) (State, error) {
	out := state20{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make20(store adt.Store) (State, error) {
	out := state20{store: store}

	s, err := market20.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state20 struct {
	market20.State
	store adt.Store
}

func (s *state20) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state20) BalancesChanged(otherState State) (bool, error) {
	otherState20, ok := otherState.(*state20)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState20.State.EscrowTable) || !s.State.LockedTable.Equals(otherState20.State.LockedTable), nil
}

func (s *state20) StatesChanged(otherState State) (bool, error) {
	otherState20, ok := otherState.(*state20)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState20.State.States), nil
}

func (s *state20) States() (DealStates, error) {
	stateArray, err := adt20.AsArray(s.store, s.State.States, market20.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates20{stateArray}, nil
}

func (s *state20) ProposalsChanged(otherState State) (bool, error) {
	otherState20, ok := otherState.(*state20)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState20.State.Proposals), nil
}

func (s *state20) Proposals() (DealProposals, error) {
	proposalArray, err := adt20.AsArray(s.store, s.State.Proposals, market20.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals20{proposalArray}, nil
}

func (s *state20) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt20.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals20{proposalCidSet}, nil
}

func (s *state20) EscrowTable() (BalanceTable, error) {
	bt, err := adt20.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable20{bt}, nil
}

func (s *state20) LockedTable() (BalanceTable, error) {
	bt, err := adt20.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable20{bt}, nil
}

func (s *state20) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market20.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state20) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable20 struct {
	*adt20.BalanceTable
}

func (bt *balanceTable20) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt20.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates20 struct {
	adt.Array
}

func (s *dealStates20) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal20 market20.DealState
	found, err := s.Array.Get(uint64(dealID), &deal20)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV20DealState(deal20)
	return deal, true, nil
}

func (s *dealStates20) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds20 market20.DealState
	return s.Array.ForEach(&ds20, func(idx int64) error {
		return cb(abi.DealID(idx), fromV20DealState(ds20))
	})
}

func (s *dealStates20) decode(val *cbg.Deferred) (DealState, error) {
	var ds20 market20.DealState
	if err := ds20.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV20DealState(ds20)
	return ds, nil
}

func (s *dealStates20) array() adt.Array {
	return s.Array
}

type dealStateV20 struct {
	ds20 market20.DealState
}

func (d dealStateV20) SectorNumber() abi.SectorNumber {

	return d.ds20.SectorNumber

}

func (d dealStateV20) SectorStartEpoch() abi.ChainEpoch {
	return d.ds20.SectorStartEpoch
}

func (d dealStateV20) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds20.LastUpdatedEpoch
}

func (d dealStateV20) SlashEpoch() abi.ChainEpoch {
	return d.ds20.SlashEpoch
}

func (d dealStateV20) Equals(other DealState) bool {
	if ov20, ok := other.(dealStateV20); ok {
		return d.ds20 == ov20.ds20
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

var _ DealState = (*dealStateV20)(nil)

func fromV20DealState(v20 market20.DealState) DealState {
	return dealStateV20{v20}
}

type dealProposals20 struct {
	adt.Array
}

func (s *dealProposals20) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal20 market20.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal20)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV20DealProposal(proposal20)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals20) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp20 market20.DealProposal
	return s.Array.ForEach(&dp20, func(idx int64) error {
		dp, err := fromV20DealProposal(dp20)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals20) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp20 market20.DealProposal
	if err := dp20.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV20DealProposal(dp20)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals20) array() adt.Array {
	return s.Array
}

type pendingProposals20 struct {
	*adt20.Set
}

func (s *pendingProposals20) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV20DealProposal(v20 market20.DealProposal) (DealProposal, error) {

	label, err := fromV20Label(v20.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v20.PieceCID,
		PieceSize:    v20.PieceSize,
		VerifiedDeal: v20.VerifiedDeal,
		Client:       v20.Client,
		Provider:     v20.Provider,

		Label: label,

		StartEpoch:           v20.StartEpoch,
		EndEpoch:             v20.EndEpoch,
		StoragePricePerEpoch: v20.StoragePricePerEpoch,

		ProviderCollateral: v20.ProviderCollateral,
		ClientCollateral:   v20.ClientCollateral,
	}, nil
}

func fromV20Label(v20 market20.DealLabel) (DealLabel, error) {
	if v20.IsString() {
		str, err := v20.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v20.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state20) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn20)(nil)

func decodePublishStorageDealsReturn20(b []byte) (PublishStorageDealsReturn, error) {
	var retval market20.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn20{retval}, nil
}

type publishStorageDealsReturn20 struct {
	market20.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn20) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn20) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state20) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {
	return verifregtypes.NoAllocationID, xerrors.Errorf("unsupported from actors v19")
}

func (s *state20) ActorKey() string {
	return manifest.MarketKey
}

func (s *state20) ActorVersion() actorstypes.Version {
	return actorstypes.Version20
}

func (s *state20) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state20) ProviderSectors() (ProviderSectors, error) {

	proverSectors, err := adt20.AsMap(s.store, s.State.ProviderSectors, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &providerSectors20{proverSectors, s.store}, nil

}

type providerSectors20 struct {
	*adt20.Map
	adt20.Store
}

type sectorDealIDs20 struct {
	*adt20.Map
}

func (s *providerSectors20) Get(actorId abi.ActorID) (SectorDealIDs, bool, error) {
	var sectorDealIdsCID cbg.CborCid
	if ok, err := s.Map.Get(abi.UIntKey(uint64(actorId)), &sectorDealIdsCID); err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	} else if !ok {
		return nil, false, nil
	}
	sectorDealIds, err := adt20.AsMap(s.Store, cid.Cid(sectorDealIdsCID), builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, false, xerrors.Errorf("failed to load sector deal ids for actor %d: %w", actorId, err)
	}
	return &sectorDealIDs20{sectorDealIds}, true, nil
}

func (s *sectorDealIDs20) ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error {
	var dealIds abi.DealIDList
	return s.Map.ForEach(&dealIds, func(key string) error {
		uk, err := abi.ParseUIntKey(key)
		if err != nil {
			return xerrors.Errorf("failed to parse sector number from key %s: %w", key, err)
		}
		return cb(abi.SectorNumber(uk), dealIds)
	})
}

func (s *sectorDealIDs20) Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error) {
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
