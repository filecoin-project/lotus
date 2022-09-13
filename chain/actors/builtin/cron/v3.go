package cron

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	cron3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/cron"
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
	out.State = *cron3.ConstructState(cron3.BuiltInEntries())
	return &out, nil
}

type state3 struct {
	cron3.State
	store adt.Store
}

func (s *state3) GetState() interface{} {
	return &s.State
}
