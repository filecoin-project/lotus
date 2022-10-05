package system

import (
	"github.com/ipfs/go-cid"

	system8 "github.com/filecoin-project/go-state-types/builtin/v8/system"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state8)(nil)

func load8(store adt.Store, root cid.Cid) (State, error) {
	out := state8{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make8(store adt.Store, builtinActors cid.Cid) (State, error) {
	out := state8{store: store}
	out.State = system8.State{
		BuiltinActors: builtinActors,
	}
	return &out, nil
}

type state8 struct {
	system8.State
	store adt.Store
}

func (s *state8) GetState() interface{} {
	return &s.State
}

func (s *state8) GetBuiltinActors() cid.Cid {

	return s.State.BuiltinActors

}

func (s *state8) SetBuiltinActors(c cid.Cid) error {

	s.State.BuiltinActors = c
	return nil

}
