package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system9 "github.com/filecoin-project/go-state-types/builtin/v9/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state9{store: store}
	out.State = system9.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state9 struct {
	system9.State
	store adt.Store
}

func (s *state9) GetState() interface{} {
	return &s.State
}

func (s *state9) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state9) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state9) ActorKey() string {
	return manifest.SystemKey
}

func (s *state9) ActorVersion() actorstypes.Version {
	return actorstypes.Version9
}

func (s *state9) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
