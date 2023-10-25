package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system13 "github.com/filecoin-project/go-state-types/builtin/v13/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state13)(nil)

func load13(store adt.Store, root cid.Cid) (State, error) {
	out := state13{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make13(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state13{store: store}
	out.State = system13.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state13 struct {
	system13.State
	store adt.Store
}

func (s *state13) GetState() interface{} {
	return &s.State
}

func (s *state13) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state13) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state13) ActorKey() string {
	return manifest.SystemKey
}

func (s *state13) ActorVersion() actorstypes.Version {
	return actorstypes.Version13
}

func (s *state13) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
