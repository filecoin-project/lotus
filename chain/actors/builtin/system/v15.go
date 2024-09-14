package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system15 "github.com/filecoin-project/go-state-types/builtin/v15/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state15)(nil)

func load15(store adt.Store, root cid.Cid) (State, error) {
	out := state15{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make15(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state15{store: store}
	out.State = system15.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state15 struct {
	system15.State
	store adt.Store
}

func (s *state15) GetState() interface{} {
	return &s.State
}

func (s *state15) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state15) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state15) ActorKey() string {
	return manifest.SystemKey
}

func (s *state15) ActorVersion() actorstypes.Version {
	return actorstypes.Version15
}

func (s *state15) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
