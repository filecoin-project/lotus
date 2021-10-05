package market

import (
	"github.com/filecoin-project/go-state-types/network"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {

	builtin.RegisterActorState(builtin0.StorageMarketActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})

	builtin.RegisterActorState(builtin2.StorageMarketActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load2(store, root)
	})

	builtin.RegisterActorState(builtin3.StorageMarketActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load3(store, root)
	})

	builtin.RegisterActorState(builtin4.StorageMarketActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load4(store, root)
	})

	builtin.RegisterActorState(builtin5.StorageMarketActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load5(store, root)
	})

	builtin.RegisterActorState(builtin6.StorageMarketActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load6(store, root)
	})
}

var (
	Address = builtin6.StorageMarketActorAddr
	Methods = builtin6.MethodsMarket
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	switch act.Code {

	case builtin0.StorageMarketActorCodeID:
		return load0(store, act.Head)

	case builtin2.StorageMarketActorCodeID:
		return load2(store, act.Head)

	case builtin3.StorageMarketActorCodeID:
		return load3(store, act.Head)

	case builtin4.StorageMarketActorCodeID:
		return load4(store, act.Head)

	case builtin5.StorageMarketActorCodeID:
		return load5(store, act.Head)

	case builtin6.StorageMarketActorCodeID:
		return load6(store, act.Head)

	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actors.Version) (State, error) {
	switch av {

	case actors.Version0:
		return make0(store)

	case actors.Version2:
		return make2(store)

	case actors.Version3:
		return make3(store)

	case actors.Version4:
		return make4(store)

	case actors.Version5:
		return make5(store)

	case actors.Version6:
		return make6(store)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

func GetActorCodeID(av actors.Version) (cid.Cid, error) {
	switch av {

	case actors.Version0:
		return builtin0.StorageMarketActorCodeID, nil

	case actors.Version2:
		return builtin2.StorageMarketActorCodeID, nil

	case actors.Version3:
		return builtin3.StorageMarketActorCodeID, nil

	case actors.Version4:
		return builtin4.StorageMarketActorCodeID, nil

	case actors.Version5:
		return builtin5.StorageMarketActorCodeID, nil

	case actors.Version6:
		return builtin6.StorageMarketActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler
	BalancesChanged(State) (bool, error)
	EscrowTable() (BalanceTable, error)
	LockedTable() (BalanceTable, error)
	TotalLocked() (abi.TokenAmount, error)
	StatesChanged(State) (bool, error)
	States() (DealStates, error)
	ProposalsChanged(State) (bool, error)
	Proposals() (DealProposals, error)
	VerifyDealsForActivation(
		minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
	) (weight, verifiedWeight abi.DealWeight, err error)
	NextID() (abi.DealID, error)
	GetState() interface{}
}

type BalanceTable interface {
	ForEach(cb func(address.Address, abi.TokenAmount) error) error
	Get(key address.Address) (abi.TokenAmount, error)
}

type DealStates interface {
	ForEach(cb func(id abi.DealID, ds DealState) error) error
	Get(id abi.DealID) (*DealState, bool, error)

	array() adt.Array
	decode(*cbg.Deferred) (*DealState, error)
}

type DealProposals interface {
	ForEach(cb func(id abi.DealID, dp DealProposal) error) error
	Get(id abi.DealID) (*DealProposal, bool, error)

	array() adt.Array
	decode(*cbg.Deferred) (*DealProposal, error)
}

type PublishStorageDealsParams = market0.PublishStorageDealsParams

type PublishStorageDealsReturn interface {
	DealIDs() ([]abi.DealID, error)
	// Note that this index is based on the batch of deals that were published, NOT the DealID
	IsDealValid(index uint64) (bool, error)
}

func DecodePublishStorageDealsReturn(b []byte, nv network.Version) (PublishStorageDealsReturn, error) {
	av, err := actors.VersionForNetwork(nv)
	if err != nil {
		return nil, err
	}

	switch av {

	case actors.Version0:
		return decodePublishStorageDealsReturn0(b)

	case actors.Version2:
		return decodePublishStorageDealsReturn2(b)

	case actors.Version3:
		return decodePublishStorageDealsReturn3(b)

	case actors.Version4:
		return decodePublishStorageDealsReturn4(b)

	case actors.Version5:
		return decodePublishStorageDealsReturn5(b)

	case actors.Version6:
		return decodePublishStorageDealsReturn6(b)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type VerifyDealsForActivationParams = market0.VerifyDealsForActivationParams
type WithdrawBalanceParams = market0.WithdrawBalanceParams

type ClientDealProposal = market0.ClientDealProposal

type DealState struct {
	SectorStartEpoch abi.ChainEpoch // -1 if not yet included in proven sector
	LastUpdatedEpoch abi.ChainEpoch // -1 if deal state never updated
	SlashEpoch       abi.ChainEpoch // -1 if deal never slashed
}

type DealProposal struct {
	PieceCID             cid.Cid
	PieceSize            abi.PaddedPieceSize
	VerifiedDeal         bool
	Client               address.Address
	Provider             address.Address
	Label                string
	StartEpoch           abi.ChainEpoch
	EndEpoch             abi.ChainEpoch
	StoragePricePerEpoch abi.TokenAmount
	ProviderCollateral   abi.TokenAmount
	ClientCollateral     abi.TokenAmount
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
	From *DealState
	To   *DealState
}

type DealProposalChanges struct {
	Added   []ProposalIDState
	Removed []ProposalIDState
}

type ProposalIDState struct {
	ID       abi.DealID
	Proposal DealProposal
}

func EmptyDealState() *DealState {
	return &DealState{
		SectorStartEpoch: -1,
		SlashEpoch:       -1,
		LastUpdatedEpoch: -1,
	}
}

// returns the earned fees and pending fees for a given deal
func (deal DealProposal) GetDealFees(height abi.ChainEpoch) (abi.TokenAmount, abi.TokenAmount) {
	tf := big.Mul(deal.StoragePricePerEpoch, big.NewInt(int64(deal.EndEpoch-deal.StartEpoch)))

	ef := big.Mul(deal.StoragePricePerEpoch, big.NewInt(int64(height-deal.StartEpoch)))
	if ef.LessThan(big.Zero()) {
		ef = big.Zero()
	}

	if ef.GreaterThan(tf) {
		ef = tf
	}

	return ef, big.Sub(tf, ef)
}
