package system

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	system8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/system"
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

func make8(store adt.Store) (State, error) {
	out := state8{store: store}
	out.State = system8.State{}
	return &out, nil
}

type state8 struct {
	system8.State
	store adt.Store
}

func (s *state8) GetState() interface{} {
	return &s.State
}
