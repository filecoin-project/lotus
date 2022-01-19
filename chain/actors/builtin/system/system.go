package system

import (
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"

	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
)

var (
	Address = builtin7.SystemActorAddr
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

	}
	return nil, xerrors.Errorf("unknown actor version %d", av)
}

func GetActorCodeID(av actors.Version) (cid.Cid, error) {
	switch av {

	case actors.Version0:
		return builtin0.SystemActorCodeID, nil

	case actors.Version2:
		return builtin2.SystemActorCodeID, nil

	case actors.Version3:
		return builtin3.SystemActorCodeID, nil

	case actors.Version4:
		return builtin4.SystemActorCodeID, nil

	case actors.Version5:
		return builtin5.SystemActorCodeID, nil

	case actors.Version6:
		return builtin6.SystemActorCodeID, nil

	case actors.Version7:
		return builtin7.SystemActorCodeID, nil

	}

	return cid.Undef, xerrors.Errorf("unknown actor version %d", av)
}

type State interface {
	GetState() interface{}
}
