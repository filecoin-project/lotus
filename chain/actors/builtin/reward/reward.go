package reward

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	reward0 "github.com/filecoin-project/specs-actors/actors/builtin/reward"
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
	Address = builtin18.RewardActorAddr
	Methods = builtin18.MethodsReward
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != manifest.RewardKey {
			return nil, xerrors.Errorf("actor code is not reward: %s", name)
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

	case builtin0.RewardActorCodeID:
		return load0(store, act.Head)

	case builtin2.RewardActorCodeID:
		return load2(store, act.Head)

	case builtin3.RewardActorCodeID:
		return load3(store, act.Head)

	case builtin4.RewardActorCodeID:
		return load4(store, act.Head)

	case builtin5.RewardActorCodeID:
		return load5(store, act.Head)

	case builtin6.RewardActorCodeID:
		return load6(store, act.Head)

	case builtin7.RewardActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actorstypes.Version, currRealizedPower abi.StoragePower) (State, error) {
	switch av {

	case actorstypes.Version0:
		return make0(store, currRealizedPower)

	case actorstypes.Version2:
		return make2(store, currRealizedPower)

	case actorstypes.Version3:
		return make3(store, currRealizedPower)

	case actorstypes.Version4:
		return make4(store, currRealizedPower)

	case actorstypes.Version5:
		return make5(store, currRealizedPower)

	case actorstypes.Version6:
		return make6(store, currRealizedPower)

	case actorstypes.Version7:
		return make7(store, currRealizedPower)

	case actorstypes.Version8:
		return make8(store, currRealizedPower)

	case actorstypes.Version9:
		return make9(store, currRealizedPower)

	case actorstypes.Version10:
		return make10(store, currRealizedPower)

	case actorstypes.Version11:
		return make11(store, currRealizedPower)

	case actorstypes.Version12:
		return make12(store, currRealizedPower)

	case actorstypes.Version13:
		return make13(store, currRealizedPower)

	case actorstypes.Version14:
		return make14(store, currRealizedPower)

	case actorstypes.Version15:
		return make15(store, currRealizedPower)

	case actorstypes.Version16:
		return make16(store, currRealizedPower)

	case actorstypes.Version17:
		return make17(store, currRealizedPower)

	case actorstypes.Version18:
		return make18(store, currRealizedPower)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	Code() cid.Cid
	ActorKey() string
	ActorVersion() actorstypes.Version

	ThisEpochBaselinePower() (abi.StoragePower, error)
	ThisEpochReward() (abi.StoragePower, error)
	ThisEpochRewardSmoothed() (builtin.FilterEstimate, error)

	EffectiveBaselinePower() (abi.StoragePower, error)
	EffectiveNetworkTime() (abi.ChainEpoch, error)

	TotalStoragePowerReward() (abi.TokenAmount, error)

	CumsumBaseline() (abi.StoragePower, error)
	CumsumRealized() (abi.StoragePower, error)

	// InitialPledgeForPower computes the pledge requirement for committing new quality-adjusted power
	// to the network, given the current network total and baseline power, per-epoch  reward, and
	// circulating token supply.
	//
	// Prior to actors version 15, the epochsSinceRampStart and rampDurationEpochs arguments have
	// no effect. After actors version 15, these values can be derived from the power actor state
	// properties RampStartEpoch and RampDurationEpochs.
	InitialPledgeForPower(qaPower abi.StoragePower, networkTotalPledge abi.TokenAmount, networkQAPower *builtin.FilterEstimate, circSupply abi.TokenAmount, epochsSinceRampStart int64, rampDurationEpochs uint64) (abi.TokenAmount, error)
	PreCommitDepositForPower(builtin.FilterEstimate, abi.StoragePower) (abi.TokenAmount, error)
	GetState() interface{}
}

type AwardBlockRewardParams = reward0.AwardBlockRewardParams

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
