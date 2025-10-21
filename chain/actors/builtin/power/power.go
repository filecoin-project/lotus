package power

import (
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	powertypes18 "github.com/filecoin-project/go-state-types/builtin/v18/power"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	Address = builtin18.StoragePowerActorAddr
	Methods = builtin18.MethodsPower
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.PowerKey {
			return nil, xerrors.Errorf("actor code is not power: %s", name)
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

	case builtin0.StoragePowerActorCodeID:
		return load0(store, act.Head)

	case builtin2.StoragePowerActorCodeID:
		return load2(store, act.Head)

	case builtin3.StoragePowerActorCodeID:
		return load3(store, act.Head)

	case builtin4.StoragePowerActorCodeID:
		return load4(store, act.Head)

	case builtin5.StoragePowerActorCodeID:
		return load5(store, act.Head)

	case builtin6.StoragePowerActorCodeID:
		return load6(store, act.Head)

	case builtin7.StoragePowerActorCodeID:
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

	TotalLocked() (abi.TokenAmount, error)
	TotalPower() (Claim, error)
	TotalCommitted() (Claim, error)
	TotalPowerSmoothed() (builtin.FilterEstimate, error)
	GetState() interface{}

	// MinerCounts returns the number of miners. Participating is the number
	// with power above the minimum miner threshold.
	MinerCounts() (participating, total uint64, err error)
	// RampStartEpoch returns the epoch at which the FIP0081 pledge calculation
	// change begins. At and before RampStartEpoch, we use the old calculation. At
	// RampStartEpoch + RampDurationEpochs, we use 70% old rules + 30% new
	// calculation.
	//
	// This method always returns 0 prior to actors version 15.
	RampStartEpoch() int64
	// RampDurationEpochs returns the number of epochs over which the new FIP0081
	// pledge calculation is ramped up.
	//
	// This method always returns 0 prior to actors version 15.
	RampDurationEpochs() uint64
	MinerPower(address.Address) (Claim, bool, error)
	MinerNominalPowerMeetsConsensusMinimum(address.Address) (bool, error)
	ListAllMiners() ([]address.Address, error)
	// ForEachClaim iterates over claims in the power actor.
	// If onlyEligible is true, it applies the MinerNominalPowerMeetsConsensusMinimum check
	// before returning the actor.
	ForEachClaim(cb func(miner address.Address, claim Claim) error, onlyEligible bool) error
	ClaimsChanged(State) (bool, error)
	CollectEligibleClaims(cacheInOut *builtin18.MapReduceCache) ([]builtin18.OwnedClaim, error)

	// Testing or genesis setup only
	SetTotalQualityAdjPower(abi.StoragePower) error
	SetTotalRawBytePower(abi.StoragePower) error
	SetThisEpochQualityAdjPower(abi.StoragePower) error
	SetThisEpochRawBytePower(abi.StoragePower) error

	// Diff helpers. Used by Diff* functions internally.
	claims() (adt.Map, error)
	decodeClaim(*cbg.Deferred) (Claim, error)
}

type Claim struct {
	// Sum of raw byte power for a miner's sectors.
	RawBytePower abi.StoragePower

	// Sum of quality adjusted power for a miner's sectors.
	QualityAdjPower abi.StoragePower
}

func AddClaims(a Claim, b Claim) Claim {
	return Claim{
		RawBytePower:    big.Add(a.RawBytePower, b.RawBytePower),
		QualityAdjPower: big.Add(a.QualityAdjPower, b.QualityAdjPower),
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

type (
	MinerPowerParams = powertypes18.MinerPowerParams
	MinerPowerReturn = powertypes18.MinerPowerReturn
)
