package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system16 "github.com/filecoin-project/go-state-types/builtin/v16/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state16)(nil)

func load16(store adt.Store, root cid.Cid) (State, error) {
	out := state16{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make16(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state16{store: store}
	out.State = system16.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state16 struct {
	system16.State
	store adt.Store
}

func (s *state16) GetState() interface{} {
	return &s.State
}

func (s *state16) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state16) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state16) ActorKey() string {
	return manifest.SystemKey
}

func (s *state16) ActorVersion() actorstypes.Version {
	return actorstypes.Version16
}

func (s *state16) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
