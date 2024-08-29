package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system14 "github.com/filecoin-project/go-state-types/builtin/v14/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state14{store: store}
	out.State = system14.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state14 struct {
	system14.State
	store adt.Store
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state14) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state14) ActorKey() string {
	return manifest.SystemKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
