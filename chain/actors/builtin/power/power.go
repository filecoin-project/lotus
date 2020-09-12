package power

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"
)

func Load(store adt.Store, act *types.Actor) (st State, err error) {
	switch act.Code {
	case v0builtin.PowerActorCodeID:
		out := v0State{store: store}
		err := store.Get(store.Context(), act.Head, &out)
		if err != nil {
			return nil, err
		}
		return &out, nil
	}
	return nil, xerrors.Errorf("unknown actor code %s", act.Code)
}

type State interface {
	cbor.Marshaler

	TotalLocked() (abi.TokenAmount, error)
}
