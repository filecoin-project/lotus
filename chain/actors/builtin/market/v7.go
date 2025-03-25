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
	market7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/market"
	adt7 "github.com/filecoin-project/specs-actors/v7/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	verifregtypes "github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store) (State, error) {
	out := state7{store: store}

	s, err := market7.ConstructState(store)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state7 struct {
	market7.State
	store adt.Store
}

func (s *state7) TotalLocked() (abi.TokenAmount, error) {
	fml := types.BigAdd(s.TotalClientLockedCollateral, s.TotalProviderLockedCollateral)
	fml = types.BigAdd(fml, s.TotalClientStorageFee)
	return fml, nil
}

func (s *state7) BalancesChanged(otherState State) (bool, error) {
	otherState7, ok := otherState.(*state7)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.EscrowTable.Equals(otherState7.State.EscrowTable) || !s.State.LockedTable.Equals(otherState7.State.LockedTable), nil
}

func (s *state7) StatesChanged(otherState State) (bool, error) {
	otherState7, ok := otherState.(*state7)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.States.Equals(otherState7.State.States), nil
}

func (s *state7) States() (DealStates, error) {
	stateArray, err := adt7.AsArray(s.store, s.State.States, market7.StatesAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealStates7{stateArray}, nil
}

func (s *state7) ProposalsChanged(otherState State) (bool, error) {
	otherState7, ok := otherState.(*state7)
	if !ok {
		// there's no way to compare different versions of the state, so let's
		// just say that means the state of balances has changed
		return true, nil
	}
	return !s.State.Proposals.Equals(otherState7.State.Proposals), nil
}

func (s *state7) Proposals() (DealProposals, error) {
	proposalArray, err := adt7.AsArray(s.store, s.State.Proposals, market7.ProposalsAmtBitwidth)
	if err != nil {
		return nil, err
	}
	return &dealProposals7{proposalArray}, nil
}

func (s *state7) PendingProposals() (PendingProposals, error) {
	proposalCidSet, err := adt7.AsSet(s.store, s.State.PendingProposals, builtin.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}
	return &pendingProposals7{proposalCidSet}, nil
}

func (s *state7) EscrowTable() (BalanceTable, error) {
	bt, err := adt7.AsBalanceTable(s.store, s.State.EscrowTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable7{bt}, nil
}

func (s *state7) LockedTable() (BalanceTable, error) {
	bt, err := adt7.AsBalanceTable(s.store, s.State.LockedTable)
	if err != nil {
		return nil, err
	}
	return &balanceTable7{bt}, nil
}

func (s *state7) VerifyDealsForActivation(
	minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
) (verifiedWeight abi.DealWeight, err error) {
	_, vw, _, err := market7.ValidateDealsForActivation(&s.State, s.store, deals, minerAddr, sectorExpiry, currEpoch)
	return vw, err
}

func (s *state7) NextID() (abi.DealID, error) {
	return s.State.NextID, nil
}

type balanceTable7 struct {
	*adt7.BalanceTable
}

func (bt *balanceTable7) ForEach(cb func(address.Address, abi.TokenAmount) error) error {
	asMap := (*adt7.Map)(bt.BalanceTable)
	var ta abi.TokenAmount
	return asMap.ForEach(&ta, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, ta)
	})
}

type dealStates7 struct {
	adt.Array
}

func (s *dealStates7) Get(dealID abi.DealID) (DealState, bool, error) {
	var deal7 market7.DealState
	found, err := s.Array.Get(uint64(dealID), &deal7)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	deal := fromV7DealState(deal7)
	return deal, true, nil
}

func (s *dealStates7) ForEach(cb func(dealID abi.DealID, ds DealState) error) error {
	var ds7 market7.DealState
	return s.Array.ForEach(&ds7, func(idx int64) error {
		return cb(abi.DealID(idx), fromV7DealState(ds7))
	})
}

func (s *dealStates7) decode(val *cbg.Deferred) (DealState, error) {
	var ds7 market7.DealState
	if err := ds7.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}
	ds := fromV7DealState(ds7)
	return ds, nil
}

func (s *dealStates7) array() adt.Array {
	return s.Array
}

type dealStateV7 struct {
	ds7 market7.DealState
}

func (d dealStateV7) SectorNumber() abi.SectorNumber {

	return 0

}

func (d dealStateV7) SectorStartEpoch() abi.ChainEpoch {
	return d.ds7.SectorStartEpoch
}

func (d dealStateV7) LastUpdatedEpoch() abi.ChainEpoch {
	return d.ds7.LastUpdatedEpoch
}

func (d dealStateV7) SlashEpoch() abi.ChainEpoch {
	return d.ds7.SlashEpoch
}

func (d dealStateV7) Equals(other DealState) bool {
	if ov7, ok := other.(dealStateV7); ok {
		return d.ds7 == ov7.ds7
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

var _ DealState = (*dealStateV7)(nil)

func fromV7DealState(v7 market7.DealState) DealState {
	return dealStateV7{v7}
}

type dealProposals7 struct {
	adt.Array
}

func (s *dealProposals7) Get(dealID abi.DealID) (*DealProposal, bool, error) {
	var proposal7 market7.DealProposal
	found, err := s.Array.Get(uint64(dealID), &proposal7)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	proposal, err := fromV7DealProposal(proposal7)
	if err != nil {
		return nil, true, xerrors.Errorf("decoding proposal: %w", err)
	}

	return &proposal, true, nil
}

func (s *dealProposals7) ForEach(cb func(dealID abi.DealID, dp DealProposal) error) error {
	var dp7 market7.DealProposal
	return s.Array.ForEach(&dp7, func(idx int64) error {
		dp, err := fromV7DealProposal(dp7)
		if err != nil {
			return xerrors.Errorf("decoding proposal: %w", err)
		}

		return cb(abi.DealID(idx), dp)
	})
}

func (s *dealProposals7) decode(val *cbg.Deferred) (*DealProposal, error) {
	var dp7 market7.DealProposal
	if err := dp7.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return nil, err
	}

	dp, err := fromV7DealProposal(dp7)
	if err != nil {
		return nil, err
	}

	return &dp, nil
}

func (s *dealProposals7) array() adt.Array {
	return s.Array
}

type pendingProposals7 struct {
	*adt7.Set
}

func (s *pendingProposals7) Has(proposalCid cid.Cid) (bool, error) {
	return s.Set.Has(abi.CidKey(proposalCid))
}

func fromV7DealProposal(v7 market7.DealProposal) (DealProposal, error) {

	label, err := labelFromGoString(v7.Label)

	if err != nil {
		return DealProposal{}, xerrors.Errorf("error setting deal label: %w", err)
	}

	return DealProposal{
		PieceCID:     v7.PieceCID,
		PieceSize:    v7.PieceSize,
		VerifiedDeal: v7.VerifiedDeal,
		Client:       v7.Client,
		Provider:     v7.Provider,

		Label: label,

		StartEpoch:           v7.StartEpoch,
		EndEpoch:             v7.EndEpoch,
		StoragePricePerEpoch: v7.StoragePricePerEpoch,

		ProviderCollateral: v7.ProviderCollateral,
		ClientCollateral:   v7.ClientCollateral,
	}, nil
}

func (s *state7) GetState() interface{} {
	return &s.State
}

var _ PublishStorageDealsReturn = (*publishStorageDealsReturn7)(nil)

func decodePublishStorageDealsReturn7(b []byte) (PublishStorageDealsReturn, error) {
	var retval market7.PublishStorageDealsReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal PublishStorageDealsReturn: %w", err)
	}

	return &publishStorageDealsReturn7{retval}, nil
}

type publishStorageDealsReturn7 struct {
	market7.PublishStorageDealsReturn
}

func (r *publishStorageDealsReturn7) IsDealValid(index uint64) (bool, int, error) {

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

func (r *publishStorageDealsReturn7) DealIDs() ([]abi.DealID, error) {
	return r.IDs, nil
}

func (s *state7) GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error) {

	return verifregtypes.NoAllocationID, xerrors.Errorf("unsupported before actors v9")

}

func (s *state7) ActorKey() string {
	return manifest.MarketKey
}

func (s *state7) ActorVersion() actorstypes.Version {
	return actorstypes.Version7
}

func (s *state7) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}

func (s *state7) ProviderSectors() (ProviderSectors, error) {

	return nil, xerrors.Errorf("unsupported before actors v13")

}
