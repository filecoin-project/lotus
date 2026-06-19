package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system19 "github.com/filecoin-project/go-state-types/builtin/v19/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state19)(nil)

func load19(store adt.Store, root cid.Cid) (State, error) {
	out := state19{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make19(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state19{store: store}
	out.State = system19.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state19 struct {
	system19.State
	store adt.Store
}

func (s *state19) GetState() interface{} {
	return &s.State
}

func (s *state19) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state19) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state19) ActorKey() string {
	return manifest.SystemKey
}

func (s *state19) ActorVersion() actorstypes.Version {
	return actorstypes.Version19
}

func (s *state19) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
