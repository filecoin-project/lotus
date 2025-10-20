package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system18 "github.com/filecoin-project/go-state-types/builtin/v18/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state18)(nil)

func load18(store adt.Store, root cid.Cid) (State, error) {
	out := state18{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make18(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state18{store: store}
	out.State = system18.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state18 struct {
	system18.State
	store adt.Store
}

func (s *state18) GetState() interface{} {
	return &s.State
}

func (s *state18) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state18) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state18) ActorKey() string {
	return manifest.SystemKey
}

func (s *state18) ActorVersion() actorstypes.Version {
	return actorstypes.Version18
}

func (s *state18) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
