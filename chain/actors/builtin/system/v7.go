package system

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	system7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/system"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store) (State, error) {
	out := state7{store: store}
	out.State = system7.State{}
	return &out, nil
}

type state7 struct {
	system7.State
	store adt.Store
}

func (s *state7) GetState() interface{} {
	return &s.State
}
