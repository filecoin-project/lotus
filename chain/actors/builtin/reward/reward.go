package reward

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/cbor"
	v0builtin "github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

var Address = v0builtin.RewardActorAddr

func Load(store adt.Store, act *types.Actor) (st State, err error) {
	switch act.Code {
	case v0builtin.RewardActorCodeID:
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

	RewardSmoothed() (builtin.FilterEstimate, error)
}
