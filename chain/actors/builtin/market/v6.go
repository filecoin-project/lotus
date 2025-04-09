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
	"github.com/filecoin-project/go-state-types/manifest"
	market6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/market"
	adt6 "github.com/filecoin-project/specs-actors/v6/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state6)(nil)

func load6(store adt.Store, root cid.Cid) (State, error) {
	out := state6{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make6(store adt.Store) (State, error) {
	out := state6{store: store}

	s, err := market6.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state6 struct {
	market6.State
	store adt.Store
}

func (s *state6) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state6) BalancesChanged(otherState State) (bool, error) {
	otherState6, ok := otherState.(*state6)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState6.State.EscrowTable) || !s.State.LockedTable.Equals(otherState6.State.LockedTable), nil
}

func (s *state6) StatesChanged(otherState State) (bool, error) {
	otherState6, ok := otherState.(*state6)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState6.State.States), nil
}

func (s *state6) States() (DealStates, error) {
	stateArray, err := adt6.AsArray(s.store, s.State.States, market6.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates6{stateArray}, nil
}

func (s *state6) ProposalsChanged(otherState State) (bool, error) {
	otherState6, ok := otherState.(*state6)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState6.State.Proposals), nil
}

func (s *state6) Proposals() (DealProposals, error) {
	proposalArray, err := adt6.AsArray(s.store, s.State.Proposals, market6.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals6{proposalArray}, nil
}

func (s *state6) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt6.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals6{proposalCidSet}, nil
}

func (s *state6) EscrowTable() (BalanceTable, error) {
	bt, err := adt6.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable6{bt}, nil
}

func (s *state6) LockedTable() (BalanceTable, error) {
	bt, err := adt6.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable6{bt}, nil
}

func (s *state6) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market6.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state6) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable6 struct {
	*adt6.BalanceTable
}

func (bt *balanceTable6) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt6.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates6 struct {
	adt.Array
}

func (s *dealStates6) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal6 market6.DealState
	found, err := s.Array.Get(uint64(dealID), &deal6)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV6DealState(deal6)
	return deal, true, nil
}

func (s *dealStates6) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds6 market6.DealState
	return s.Array.ForEach(&ds6, func(idx int64) error {
		return cb(abi.DealID(idx), fromV6DealState(ds6))
	})
}

func (s *dealStates6) decode(val *cbg.Deferred) (DealState, error) {
	var ds6 market6.DealState
	if err := ds6.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV6DealState(ds6)
	return ds, nil
}

func (s *dealStates6) array() adt.Array {
	return s.Array
}

type dealStateV6 struct {
	ds6 market6.DealState
}

func (d dealStateV6) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV6) SectorStartEpoch() abi.ChainEpoch {
	return d.ds6.SectorStartEpoch
}

func (d dealStateV6) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds6.LastUpdatedEpoch
}

func (d dealStateV6) SlashEpoch() abi.ChainEpoch {
	return d.ds6.SlashEpoch
}

func (d dealStateV6) Equals(other DealState) bool {
	if ov6, ok := other.(dealStateV6); ok {
		return d.ds6 == ov6.ds6
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

var _ DealState = (*dealStateV6)(nil)

func fromV6DealState(v6 market6.DealState) DealState {
	return dealStateV6{v6}
}

type dealProposals6 struct {
	adt.Array
}

func (s *dealProposals6) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal6 market6.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal6)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV6DealProposal(proposal6)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals6) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp6 market6.DealProposal
	return s.Array.ForEach(&dp6, func(idx int64) error {
		dp, err := fromV6DealProposal(dp6)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals6) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp6 market6.DealProposal
	if err := dp6.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV6DealProposal(dp6)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals6) array() adt.Array {
	return s.Array
}

type pendingProposals6 struct {
	*adt6.Set
}

func (s *pendingProposals6) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV6DealProposal(v6 market6.DealProposal) (DealProposal, error) {

	label, err := labelFromGoString(v6.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v6.PieceCID,
		PieceSize:    v6.PieceSize,
		VerifiedDeal: v6.VerifiedDeal,
		Client:       v6.Client,
		Provider:     v6.Provider,

		Label: label,

		StartEpoch:           v6.StartEpoch,
		EndEpoch:             v6.EndEpoch,
		StoragePricePerEpoch: v6.StoragePricePerEpoch,

		ProviderCollateral: v6.ProviderCollateral,
		ClientCollateral:   v6.ClientCollateral,
	}, nil
}

func (s *state6) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn6)(nil)

func decodePublishStorageDealsReturn6(b []byte) (PublishStorageDealsReturn, error) {
	var retval market6.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn6{retval}, nil
}

type publishStorageDealsReturn6 struct {
	market6.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn6) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn6) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state6) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	return verifregtypes.NoAllocationID, xerrors.Errorf("unsupported before actors v9")

}

func (s *state6) ActorKey() string {
	return manifest.MarketKey
}

func (s *state6) ActorVersion() actorstypes.Version {
	return actorstypes.Version6
}

func (s *state6) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state6) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
