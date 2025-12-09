package market

import (
	"unicode/utf8"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	Address = builtintypes.StorageMarketActorAddr
	Methods = builtintypes.MethodsMarket
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.MarketKey {
			return nil, xerrors.Errorf("actor code is not market: %s", name)
		}

		switch av {

		case actorstypes.Version8:
			return load8(store, act.Head)

		case actorstypes.Version9:
			return load9(store, act.Head)

		case actorstypes.Version10:
			return load10(store, act.Head)

		case actorstypes.Version11:
			return load11(store, act.Head)

		case actorstypes.Version12:
			return load12(store, act.Head)

		case actorstypes.Version13:
			return load13(store, act.Head)

		case actorstypes.Version14:
			return load14(store, act.Head)

		case actorstypes.Version15:
			return load15(store, act.Head)

		case actorstypes.Version16:
			return load16(store, act.Head)

		case actorstypes.Version17:
			return load17(store, act.Head)

		case actorstypes.Version18:
			return load18(store, act.Head)

		}
	}

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

	case builtin7.StorageMarketActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version) (State, error) {
	switch av {

	case actorstypes.Version0:
		return make0(store)

	case actorstypes.Version2:
		return make2(store)

	case actorstypes.Version3:
		return make3(store)

	case actorstypes.Version4:
		return make4(store)

	case actorstypes.Version5:
		return make5(store)

	case actorstypes.Version6:
		return make6(store)

	case actorstypes.Version7:
		return make7(store)

	case actorstypes.Version8:
		return make8(store)

	case actorstypes.Version9:
		return make9(store)

	case actorstypes.Version10:
		return make10(store)

	case actorstypes.Version11:
		return make11(store)

	case actorstypes.Version12:
		return make12(store)

	case actorstypes.Version13:
		return make13(store)

	case actorstypes.Version14:
		return make14(store)

	case actorstypes.Version15:
		return make15(store)

	case actorstypes.Version16:
		return make16(store)

	case actorstypes.Version17:
		return make17(store)

	case actorstypes.Version18:
		return make18(store)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

	BalancesChanged(State) (bool, error)
	EscrowTable() (BalanceTable, error)
	LockedTable() (BalanceTable, error)
	TotalLocked() (abi.TokenAmount, error)
	StatesChanged(State) (bool, error)
	States() (DealStates, error)
	ProposalsChanged(State) (bool, error)
	Proposals() (DealProposals, error)
	PendingProposals() (PendingProposals, error)
	VerifyDealsForActivation(
		minerAddr address.Address, deals []abi.DealID, currEpoch, sectorExpiry abi.ChainEpoch,
	) (verifiedWeight abi.DealWeight, err error)
	NextID() (abi.DealID, error)
	GetState() interface{}
	GetAllocationIdForPendingDeal(dealId abi.DealID) (verifregtypes.AllocationId, error)
	ProviderSectors() (ProviderSectors, error)
}

type BalanceTable interface {
	ForEach(cb func(address.Address, abi.TokenAmount) error) error
	Get(key address.Address) (abi.TokenAmount, error)
}

type DealStates interface {
	ForEach(cb func(id abi.DealID, ds DealState) error) error
	Get(id abi.DealID) (DealState, bool, error)

	array() adt.Array
	decode(*cbg.Deferred) (DealState, error)
}

type DealProposals interface {
	ForEach(cb func(id abi.DealID, dp markettypes.DealProposal) error) error
	Get(id abi.DealID) (*markettypes.DealProposal, bool, error)

	array() adt.Array
	decode(*cbg.Deferred) (*markettypes.DealProposal, error)
}

type PendingProposals interface {
	Has(proposalCid cid.Cid) (bool, error)
}

type PublishStorageDealsReturn interface {
	DealIDs() ([]abi.DealID, error)
	// Note that this index is based on the batch of deals that were published, NOT the DealID
	IsDealValid(index uint64) (bool, int, error)
}

func DecodePublishStorageDealsReturn(b []byte, nv network.Version) (PublishStorageDealsReturn, error) {
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, err
	}

	switch av {

	case actorstypes.Version0:
		return decodePublishStorageDealsReturn0(b)

	case actorstypes.Version2:
		return decodePublishStorageDealsReturn2(b)

	case actorstypes.Version3:
		return decodePublishStorageDealsReturn3(b)

	case actorstypes.Version4:
		return decodePublishStorageDealsReturn4(b)

	case actorstypes.Version5:
		return decodePublishStorageDealsReturn5(b)

	case actorstypes.Version6:
		return decodePublishStorageDealsReturn6(b)

	case actorstypes.Version7:
		return decodePublishStorageDealsReturn7(b)

	case actorstypes.Version8:
		return decodePublishStorageDealsReturn8(b)

	case actorstypes.Version9:
		return decodePublishStorageDealsReturn9(b)

	case actorstypes.Version10:
		return decodePublishStorageDealsReturn10(b)

	case actorstypes.Version11:
		return decodePublishStorageDealsReturn11(b)

	case actorstypes.Version12:
		return decodePublishStorageDealsReturn12(b)

	case actorstypes.Version13:
		return decodePublishStorageDealsReturn13(b)

	case actorstypes.Version14:
		return decodePublishStorageDealsReturn14(b)

	case actorstypes.Version15:
		return decodePublishStorageDealsReturn15(b)

	case actorstypes.Version16:
		return decodePublishStorageDealsReturn16(b)

	case actorstypes.Version17:
		return decodePublishStorageDealsReturn17(b)

	case actorstypes.Version18:
		return decodePublishStorageDealsReturn18(b)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type DealProposal = markettypes.DealProposal
type DealLabel = markettypes.DealLabel

type DealState interface {
	SectorNumber() abi.SectorNumber   // 0 if not yet included in proven sector (0 is also a valid sector number)
	SectorStartEpoch() abi.ChainEpoch // -1 if not yet included in proven sector
	LastUpdatedEpoch() abi.ChainEpoch // -1 if deal state never updated
	SlashEpoch() abi.ChainEpoch       // -1 if deal never slashed

	Equals(other DealState) bool
}

type ProviderSectors interface {
	Get(actorId abi.ActorID) (SectorDealIDs, bool, error)
}

type SectorDealIDs interface {
	ForEach(cb func(abi.SectorNumber, []abi.DealID) error) error
	Get(sectorNumber abi.SectorNumber) ([]abi.DealID, bool, error)
}

func DealStatesEqual(a, b DealState) bool {
	if a.SectorNumber() != b.SectorNumber() {
		return false
	}
	if a.SectorStartEpoch() != b.SectorStartEpoch() {
		return false
	}
	if a.LastUpdatedEpoch() != b.LastUpdatedEpoch() {
		return false
	}
	if a.SlashEpoch() != b.SlashEpoch() {
		return false
	}
	return true
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
	Proposal markettypes.DealProposal
}

type emptyDealState struct{}

func (e *emptyDealState) SectorNumber() abi.SectorNumber {
	return 0
}

func (e *emptyDealState) SectorStartEpoch() abi.ChainEpoch {
	return -1
}

func (e *emptyDealState) LastUpdatedEpoch() abi.ChainEpoch {
	return -1
}

func (e *emptyDealState) SlashEpoch() abi.ChainEpoch {
	return -1
}

func (e *emptyDealState) Equals(other DealState) bool {
	return DealStatesEqual(e, other)
}

func EmptyDealState() DealState {
	return &emptyDealState{}
}

// returns the earned fees and pending fees for a given deal
func GetDealFees(deal markettypes.DealProposal, height abi.ChainEpoch) (abi.TokenAmount, abi.TokenAmount) {
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

func IsDealActive(state DealState) bool {
	return state.SectorStartEpoch() > -1 && state.SlashEpoch() == -1
}

func labelFromGoString(s string) (markettypes.DealLabel, error) {
	if utf8.ValidString(s) {
		return markettypes.NewLabelFromString(s)
	} else {
		return markettypes.NewLabelFromBytes([]byte(s))
	}
}

func AllCodes() []cid.Cid {
	return []cid.Cid{
		(&state0{}).Code(),
		(&state2{}).Code(),
		(&state3{}).Code(),
		(&state4{}).Code(),
		(&state5{}).Code(),
		(&state6{}).Code(),
		(&state7{}).Code(),
		(&state8{}).Code(),
		(&state9{}).Code(),
		(&state10{}).Code(),
		(&state11{}).Code(),
		(&state12{}).Code(),
		(&state13{}).Code(),
		(&state14{}).Code(),
		(&state15{}).Code(),
		(&state16{}).Code(),
		(&state17{}).Code(),
		(&state18{}).Code(),
	}
}
