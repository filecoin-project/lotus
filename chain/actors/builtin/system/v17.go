package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system17 "github.com/filecoin-project/go-state-types/builtin/v17/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state17)(nil)

func load17(store adt.Store, root cid.Cid) (State, error) {
	out := state17{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make17(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state17{store: store}
	out.State = system17.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state17 struct {
	system17.State
	store adt.Store
}

func (s *state17) GetState() interface{} {
	return &s.State
}

func (s *state17) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state17) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state17) ActorKey() string {
	return manifest.SystemKey
}

func (s *state17) ActorVersion() actorstypes.Version {
	return actorstypes.Version17
}

func (s *state17) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
