package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors"
	reward0 "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/cbor"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	builtin8 "github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	Address = builtin8.RewardActorAddr
	Methods = builtin8.MethodsReward
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != actors.RewardKey {
			return nil, xerrors.Errorf("actor code is not reward: %s", name)
		}

		switch av {

		case actors.Version8:
			return load8(store, act.Head)

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

func MakeState(store adt.Store, av actors.Version, currRealizedPower abi.StoragePower) (State, error) {
	switch av {

	case actors.Version0:
		return make0(store, currRealizedPower)

	case actors.Version2:
		return make2(store, currRealizedPower)

	case actors.Version3:
		return make3(store, currRealizedPower)

	case actors.Version4:
		return make4(store, currRealizedPower)

	case actors.Version5:
		return make5(store, currRealizedPower)

	case actors.Version6:
		return make6(store, currRealizedPower)

	case actors.Version7:
		return make7(store, currRealizedPower)

	case actors.Version8:
		return make8(store, currRealizedPower)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	cbor.Marshaler

	ThisEpochBaselinePower() (abi.StoragePower, error)
	ThisEpochReward() (abi.StoragePower, error)
	ThisEpochRewardSmoothed() (builtin.FilterEstimate, error)

	EffectiveBaselinePower() (abi.StoragePower, error)
	EffectiveNetworkTime() (abi.ChainEpoch, error)

	TotalStoragePowerReward() (abi.TokenAmount, error)

	CumsumBaseline() (abi.StoragePower, error)
	CumsumRealized() (abi.StoragePower, error)

	InitialPledgeForPower(abi.StoragePower, abi.TokenAmount, *builtin.FilterEstimate, abi.TokenAmount) (abi.TokenAmount, error)
	PreCommitDepositForPower(builtin.FilterEstimate, abi.StoragePower) (abi.TokenAmount, error)
	GetState() interface{}
}

type AwardBlockRewardParams = reward0.AwardBlockRewardParams
