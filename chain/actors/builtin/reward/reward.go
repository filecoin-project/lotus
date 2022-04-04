package reward

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors"
	reward0 "github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/cbor"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {

	builtin.RegisterActorState(builtin0.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load0(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version0, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load0(store, root)
		})
	}

	builtin.RegisterActorState(builtin2.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load2(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version2, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load2(store, root)
		})
	}

	builtin.RegisterActorState(builtin3.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load3(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version3, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load3(store, root)
		})
	}

	builtin.RegisterActorState(builtin4.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load4(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version4, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load4(store, root)
		})
	}

	builtin.RegisterActorState(builtin5.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load5(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version5, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load5(store, root)
		})
	}

	builtin.RegisterActorState(builtin6.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load6(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version6, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load6(store, root)
		})
	}

	builtin.RegisterActorState(builtin7.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load7(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version7, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load7(store, root)
		})
	}

	builtin.RegisterActorState(builtin8.RewardActorCodeID, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
		return load8(store, root)
	})

	if c, ok := actors.GetActorCodeID(actors.Version8, "reward"); ok {
		builtin.RegisterActorState(c, func(store adt.Store, root cid.Cid) (cbor.Marshaler, error) {
			return load8(store, root)
		})
	}
}

var (
	Address = builtin8.RewardActorAddr
	Methods = builtin8.MethodsReward
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != "reward" {
			return nil, xerrors.Errorf("actor code is not reward: %s", name)
		}

		switch av {

		case actors.Version0:
			return load0(store, act.Head)

		case actors.Version2:
			return load2(store, act.Head)

		case actors.Version3:
			return load3(store, act.Head)

		case actors.Version4:
			return load4(store, act.Head)

		case actors.Version5:
			return load5(store, act.Head)

		case actors.Version6:
			return load6(store, act.Head)

		case actors.Version7:
			return load7(store, act.Head)

		case actors.Version8:
			return load8(store, act.Head)

		default:
			return nil, xerrors.Errorf("unknown actor version: %d", av)
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

	case builtin8.RewardActorCodeID:
		return load8(store, act.Head)

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

func GetActorCodeID(av actors.Version) (cid.Cid, error) {
	if c, ok := actors.GetActorCodeID(av, "reward"); ok {
		return c, nil
	}

	switch av {

	case actors.Version0:
		return builtin0.RewardActorCodeID, nil

	case actors.Version2:
		return builtin2.RewardActorCodeID, nil

	case actors.Version3:
		return builtin3.RewardActorCodeID, nil

	case actors.Version4:
		return builtin4.RewardActorCodeID, nil

	case actors.Version5:
		return builtin5.RewardActorCodeID, nil

	case actors.Version6:
		return builtin6.RewardActorCodeID, nil

	case actors.Version7:
		return builtin7.RewardActorCodeID, nil

	case actors.Version8:
		return builtin8.RewardActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
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
