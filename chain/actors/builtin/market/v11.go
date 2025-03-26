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
	market11 "github.com/filecoin-project/go-state-types/builtin/v11/market"
	adt11 "github.com/filecoin-project/go-state-types/builtin/v11/util/adt"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state11)(nil)

func load11(store adt.Store, root cid.Cid) (State, error) {
	out := state11{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make11(store adt.Store) (State, error) {
	out := state11{store: store}

	s, err := market11.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state11 struct {
	market11.State
	store adt.Store
}

func (s *state11) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state11) BalancesChanged(otherState State) (bool, error) {
	otherState11, ok := otherState.(*state11)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState11.State.EscrowTable) || !s.State.LockedTable.Equals(otherState11.State.LockedTable), nil
}

func (s *state11) StatesChanged(otherState State) (bool, error) {
	otherState11, ok := otherState.(*state11)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState11.State.States), nil
}

func (s *state11) States() (DealStates, error) {
	stateArray, err := adt11.AsArray(s.store, s.State.States, market11.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates11{stateArray}, nil
}

func (s *state11) ProposalsChanged(otherState State) (bool, error) {
	otherState11, ok := otherState.(*state11)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState11.State.Proposals), nil
}

func (s *state11) Proposals() (DealProposals, error) {
	proposalArray, err := adt11.AsArray(s.store, s.State.Proposals, market11.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals11{proposalArray}, nil
}

func (s *state11) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt11.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals11{proposalCidSet}, nil
}

func (s *state11) EscrowTable() (BalanceTable, error) {
	bt, err := adt11.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable11{bt}, nil
}

func (s *state11) LockedTable() (BalanceTable, error) {
	bt, err := adt11.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable11{bt}, nil
}

func (s *state11) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market11.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state11) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable11 struct {
	*adt11.BalanceTable
}

func (bt *balanceTable11) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt11.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates11 struct {
	adt.Array
}

func (s *dealStates11) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal11 market11.DealState
	found, err := s.Array.Get(uint64(dealID), &deal11)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV11DealState(deal11)
	return deal, true, nil
}

func (s *dealStates11) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds11 market11.DealState
	return s.Array.ForEach(&ds11, func(idx int64) error {
		return cb(abi.DealID(idx), fromV11DealState(ds11))
	})
}

func (s *dealStates11) decode(val *cbg.Deferred) (DealState, error) {
	var ds11 market11.DealState
	if err := ds11.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV11DealState(ds11)
	return ds, nil
}

func (s *dealStates11) array() adt.Array {
	return s.Array
}

type dealStateV11 struct {
	ds11 market11.DealState
}

func (d dealStateV11) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV11) SectorStartEpoch() abi.ChainEpoch {
	return d.ds11.SectorStartEpoch
}

func (d dealStateV11) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds11.LastUpdatedEpoch
}

func (d dealStateV11) SlashEpoch() abi.ChainEpoch {
	return d.ds11.SlashEpoch
}

func (d dealStateV11) Equals(other DealState) bool {
	if ov11, ok := other.(dealStateV11); ok {
		return d.ds11 == ov11.ds11
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

var _ DealState = (*dealStateV11)(nil)

func fromV11DealState(v11 market11.DealState) DealState {
	return dealStateV11{v11}
}

type dealProposals11 struct {
	adt.Array
}

func (s *dealProposals11) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal11 market11.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal11)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV11DealProposal(proposal11)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals11) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp11 market11.DealProposal
	return s.Array.ForEach(&dp11, func(idx int64) error {
		dp, err := fromV11DealProposal(dp11)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals11) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp11 market11.DealProposal
	if err := dp11.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV11DealProposal(dp11)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals11) array() adt.Array {
	return s.Array
}

type pendingProposals11 struct {
	*adt11.Set
}

func (s *pendingProposals11) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV11DealProposal(v11 market11.DealProposal) (DealProposal, error) {

	label, err := fromV11Label(v11.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v11.PieceCID,
		PieceSize:    v11.PieceSize,
		VerifiedDeal: v11.VerifiedDeal,
		Client:       v11.Client,
		Provider:     v11.Provider,

		Label: label,

		StartEpoch:           v11.StartEpoch,
		EndEpoch:             v11.EndEpoch,
		StoragePricePerEpoch: v11.StoragePricePerEpoch,

		ProviderCollateral: v11.ProviderCollateral,
		ClientCollateral:   v11.ClientCollateral,
	}, nil
}

func fromV11Label(v11 market11.DealLabel) (DealLabel, error) {
	if v11.IsString() {
		str, err := v11.ToString()
		if err != nil {
			return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert string label to string: %w", err)
		}
		return markettypes.NewLabelFromString(str)
	}

	bs, err := v11.ToBytes()
	if err != nil {
		return markettypes.EmptyDealLabel, xerrors.Errorf("failed to convert bytes label to bytes: %w", err)
	}
	return markettypes.NewLabelFromBytes(bs)
}

func (s *state11) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn11)(nil)

func decodePublishStorageDealsReturn11(b []byte) (PublishStorageDealsReturn, error) {
	var retval market11.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn11{retval}, nil
}

type publishStorageDealsReturn11 struct {
	market11.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn11) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn11) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state11) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	allocations, err := adt11.AsMap(s.store, s.PendingDealAllocationIds, builtin.DefaultHamtBitwidth)
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

func (s *state11) ActorKey() string {
	return manifest.MarketKey
}

func (s *state11) ActorVersion() actorstypes.Version {
	return actorstypes.Version11
}

func (s *state11) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state11) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
