package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system12 "github.com/filecoin-project/go-state-types/builtin/v12/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state12)(nil)

func load12(store adt.Store, root cid.Cid) (State, error) {
	out := state12{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make12(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state12{store: store}
	out.State = system12.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state12 struct {
	system12.State
	store adt.Store
}

func (s *state12) GetState() interface{} {
	return &s.State
}

func (s *state12) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state12) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state12) ActorKey() string {
	return manifest.SystemKey
}

func (s *state12) ActorVersion() actorstypes.Version {
	return actorstypes.Version12
}

func (s *state12) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
