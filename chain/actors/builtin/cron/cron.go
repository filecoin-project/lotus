package cron

import (
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin9 "github.com/filecoin-project/go-state-types/builtin"
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

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != actors.CronKey {
			return nil, xerrors.Errorf("actor code is not cron: %s", name)
		}

		switch av {

		case actorstypes.Version8:
			return load8(store, act.Head)

		case actorstypes.Version9:
			return load9(store, act.Head)

		}
	}

	switch act.Code {

	case builtin0.CronActorCodeID:
		return load0(store, act.Head)

	case builtin2.CronActorCodeID:
		return load2(store, act.Head)

	case builtin3.CronActorCodeID:
		return load3(store, act.Head)

	case builtin4.CronActorCodeID:
		return load4(store, act.Head)

	case builtin5.CronActorCodeID:
		return load5(store, act.Head)

	case builtin6.CronActorCodeID:
		return load6(store, act.Head)

	case builtin7.CronActorCodeID:
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

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

var (
	Address = builtin9.CronActorAddr
	Methods = builtin9.MethodsCron
)

type State interface {
	GetState() interface{}
}
