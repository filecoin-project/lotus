package cron

import (
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"golang.org/x/xerrors"
	"github.com/ipfs/go-cid"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"

	builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"

)

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

	case actors.Version7:
		return make7(store)

	case actors.Version8:
		return make8(store)

}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

func GetActorCodeID(av actors.Version) (cid.Cid, error) {
    if c, ok := actors.GetActorCodeID(av, "cron"); ok {
       return c, nil
    }

	switch av {

	case actors.Version0:
		return builtin0.CronActorCodeID, nil

	case actors.Version2:
		return builtin2.CronActorCodeID, nil

	case actors.Version3:
		return builtin3.CronActorCodeID, nil

	case actors.Version4:
		return builtin4.CronActorCodeID, nil

	case actors.Version5:
		return builtin5.CronActorCodeID, nil

	case actors.Version6:
		return builtin6.CronActorCodeID, nil

	case actors.Version7:
		return builtin7.CronActorCodeID, nil

	case actors.Version8:
		return builtin8.CronActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

var (
	Address = builtin8.CronActorAddr
	Methods = builtin8.MethodsCron
)


type State interface {
	GetState() interface{}
}
