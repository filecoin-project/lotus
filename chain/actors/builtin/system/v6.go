package system

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	system6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/system"
)

var _ State = (*state6)(nil)

func load6(store adt.Store, root cid.Cid) (State, error) {
	out := state6{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make6(store adt.Store) (State, error) {
	out := state6{store: store}
	out.State = system6.State{}
	return &out, nil
}

type state6 struct {
	system6.State
	store adt.Store
}

func (s *state6) GetState() interface{} {
	return &s.State
}
