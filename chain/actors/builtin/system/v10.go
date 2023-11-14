package system

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system10 "github.com/filecoin-project/go-state-types/builtin/v10/system"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state10{store: store}
	out.State = system10.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state10 struct {
	system10.State
	store adt.Store
}

func (s *state10) GetState() interface{} {
	return &s.State
}

func (s *state10) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state10) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}

func (s *state10) ActorKey() string {
	return manifest.SystemKey
}

func (s *state10) ActorVersion() actorstypes.Version {
	return actorstypes.Version10
}

func (s *state10) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
