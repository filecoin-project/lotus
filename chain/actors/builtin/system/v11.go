package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system11 "github.com/filecoin-project/go-state-types/builtin/v11/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state11)(nil)

func load11(store adt.Store, root cid.Cid) (State, error) {
	out := state11{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make11(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state11{store: store}
	out.State = system11.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state11 struct {
	system11.State
	store adt.Store
}

func (s *state11) GetState() interface{} {
	return &s.State
}

func (s *state11) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state11) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state11) ActorKey() string {
	return manifest.SystemKey
}

func (s *state11) ActorVersion() actorstypes.Version {
	return actorstypes.Version11
}

func (s *state11) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
