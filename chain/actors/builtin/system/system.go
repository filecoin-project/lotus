package system

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	builtin8 "github.com/filecoin-project/go-state-types/builtin"
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
	Address = builtin8.SystemActorAddr
)

func Load(store adt.Store, act *types.Actor) (State, error) {
	if name, av, ok := actors.GetActorMetaByCode(act.Code); ok {
		if name != actors.SystemKey {
			return nil, xerrors.Errorf("actor code is not system: %s", name)
		}

		switch av {

		case actors.Version8:
			return load8(store, act.Head)

		}
	}

	switch act.Code {

	case builtin0.SystemActorCodeID:
		return load0(store, act.Head)

	case builtin2.SystemActorCodeID:
		return load2(store, act.Head)

	case builtin3.SystemActorCodeID:
		return load3(store, act.Head)

	case builtin4.SystemActorCodeID:
		return load4(store, act.Head)

	case builtin5.SystemActorCodeID:
		return load5(store, act.Head)

	case builtin6.SystemActorCodeID:
		return load6(store, act.Head)

	case builtin7.SystemActorCodeID:
		return load7(store, act.Head)

	}

	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

func MakeState(store adt.Store, av actors.Version, builtinActors cid.Cid) (State, error) {
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

	case actors.Version7:
		return make7(store)

	case actors.Version8:
		return make8(store, builtinActors)

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	GetState() interface{}
	GetBuiltinActors() cid.Cid
}
