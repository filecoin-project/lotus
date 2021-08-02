package system

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	system3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/system"
)

var _ State = (*state3)(nil)

func load3(store adt.Store, root cid.Cid) (State, error) {
	out := state3{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make3(store adt.Store) (State, error) {
	out := state3{store: store}
	out.State = system3.State{}
	return &out, nil
}

type state3 struct {
	system3.State
	store adt.Store
}

func (s *state3) GetState() interface{} {
	return &s.State
}
