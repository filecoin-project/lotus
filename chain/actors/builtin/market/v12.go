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
	market12 "github.com/filecoin-project/go-state-types/builtin/v12/market"
	adt12 "github.com/filecoin-project/go-state-types/builtin/v12/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state12)(nil)

func load12(store adt.Store, root cid.Cid) (State, error) {
	out := state12{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make12(store adt.Store) (State, error) {
	out := state12{store: store}

	s, err := market12.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state12 struct {
	market12.State
	store adt.Store
}

func (s *state12) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state12) BalancesChanged(otherState State) (bool, error) {
	otherState12, ok := otherState.(*state12)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState12.State.EscrowTable) || !s.State.LockedTable.Equals(otherState12.State.LockedTable), nil
}

func (s *state12) StatesChanged(otherState State) (bool, error) {
	otherState12, ok := otherState.(*state12)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState12.State.States), nil
}

func (s *state12) States() (DealStates, error) {
	stateArray, err := adt12.AsArray(s.store, s.State.States, market12.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates12{stateArray}, nil
}

func (s *state12) ProposalsChanged(otherState State) (bool, error) {
	otherState12, ok := otherState.(*state12)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState12.State.Proposals), nil
}

func (s *state12) Proposals() (DealProposals, error) {
	proposalArray, err := adt12.AsArray(s.store, s.State.Proposals, market12.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals12{proposalArray}, nil
}

func (s *state12) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt12.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals12{proposalCidSet}, nil
}

func (s *state12) EscrowTable() (BalanceTable, error) {
	bt, err := adt12.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable12{bt}, nil
}

func (s *state12) LockedTable() (BalanceTable, error) {
	bt, err := adt12.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable12{bt}, nil
}

func (s *state12) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market12.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state12) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable12 struct {
	*adt12.BalanceTable
}

func (bt *balanceTable12) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt12.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates12 struct {
	adt.Array
}

func (s *dealStates12) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal12 market12.DealState
	found, err := s.Array.Get(uint64(dealID), &deal12)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV12DealState(deal12)
	return deal, true, nil
}

func (s *dealStates12) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds12 market12.DealState
	return s.Array.ForEach(&ds12, func(idx int64) error {
		return cb(abi.DealID(idx), fromV12DealState(ds12))
	})
}

func (s *dealStates12) decode(val *cbg.Deferred) (DealState, error) {
	var ds12 market12.DealState
	if err := ds12.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV12DealState(ds12)
	return ds, nil
}

func (s *dealStates12) array() adt.Array {
	return s.Array
}

type dealStateV12 struct {
	ds12 market12.DealState
}

func (d dealStateV12) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV12) SectorStartEpoch() abi.ChainEpoch {
	return d.ds12.SectorStartEpoch
}

func (d dealStateV12) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds12.LastUpdatedEpoch
}

func (d dealStateV12) SlashEpoch() abi.ChainEpoch {
	return d.ds12.SlashEpoch
}

func (d dealStateV12) Equals(other DealState) bool {
	if ov12, ok := other.(dealStateV12); ok {
		return d.ds12 == ov12.ds12
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

var _ DealState = (*dealStateV12)(nil)

func fromV12DealState(v12 market12.DealState) DealState {
	return dealStateV12{v12}
}

type dealProposals12 struct {
	adt.Array
}

func (s *dealProposals12) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal12 market12.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal12)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV12DealProposal(proposal12)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals12) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp12 market12.DealProposal
	return s.Array.ForEach(&dp12, func(idx int64) error {
		dp, err := fromV12DealProposal(dp12)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals12) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp12 market12.DealProposal
	if err := dp12.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV12DealProposal(dp12)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals12) array() adt.Array {
	return s.Array
}

type pendingProposals12 struct {
	*adt12.Set
}

func (s *pendingProposals12) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV12DealProposal(v12 market12.DealProposal) (DealProposal, error) {

	label, err := fromV12Label(v12.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v12.PieceCID,
		PieceSize:    v12.PieceSize,
		VerifiedDeal: v12.VerifiedDeal,
		Client:       v12.Client,
		Provider:     v12.Provider,

		Label: label,

		StartEpoch:           v12.StartEpoch,
		EndEpoch:             v12.EndEpoch,
		StoragePricePerEpoch: v12.StoragePricePerEpoch,

		ProviderCollateral: v12.ProviderCollateral,
		ClientCollateral:   v12.ClientCollateral,
	}, nil
}

func fromV12Label(v12 market12.DealLabel) (DealLabel, error) {
	if v12.IsString() {
		str, err := v12.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v12.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state12) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn12)(nil)

func decodePublishStorageDealsReturn12(b []byte) (PublishStorageDealsReturn, error) {
	var retval market12.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn12{retval}, nil
}

type publishStorageDealsReturn12 struct {
	market12.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn12) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn12) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state12) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt12.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state12) ActorKey() string {
	return manifest.MarketKey
}

func (s *state12) ActorVersion() actorstypes.Version {
	return actorstypes.Version12
}

func (s *state12) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state12) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
