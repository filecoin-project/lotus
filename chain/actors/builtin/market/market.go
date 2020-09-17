package market

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	v0builtin "github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

var Address = v0builtin.StorageMarketActorAddr

func Load(store adt.Store, act *types.Actor) (st State, err error) {
	switch act.Code {
	case v0builtin.StorageMarketActorCodeID:
		out := v0State{store: store}
		err := store.Get(store.Context(), act.Head, &out)
		if err != nil {
			return nil, err
		}
		return &out, nil
	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler
	BalancesChanged(State) bool
	EscrowTable() (BalanceTable, error)
	LockedTable() (BalanceTable, error)
	TotalLocked() (abi.TokenAmount, error)
	StatesChanged(State) bool
	States() (DealStates, error)
	ProposalsChanged(State) bool
	Proposals() (DealProposals, error)
	VerifyDealsForActivation(
		minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
	) (weight, verifiedWeight abi.DealWeight, err error)
}

type BalanceTable interface {
	Get(key address.Address) (abi.TokenAmount, error)
}

type DealStates interface {
	GetDeal(key abi.DealID) (DealState, error)
	Diff(DealStates) (*DealStateChanges, error)
}

type DealProposals interface {
	Diff(DealProposals) (*DealProposalChanges, error)
}

type DealState interface {
	SectorStartEpoch() abi.ChainEpoch
	SlashEpoch() abi.ChainEpoch
	LastUpdatedEpoch() abi.ChainEpoch
	Equals(DealState) bool
}

type DealProposal interface {
}

type DealStateChanges struct {
	Added    []DealIDState
	Modified []DealStateChange
	Removed  []DealIDState
}

type DealIDState struct {
	ID   abi.DealID
	Deal DealState
}

// DealStateChange is a change in deal state from -> to
type DealStateChange struct {
	ID   abi.DealID
	From DealState
	To   DealState
}

type DealProposalChanges struct {
	Added   []ProposalIDState
	Removed []ProposalIDState
}

type ProposalIDState struct {
	ID       abi.DealID
	Proposal DealProposal
}
