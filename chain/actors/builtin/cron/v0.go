package cron

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	cron0 "github.com/filecoin-project/specs-actors/actors/builtin/cron"
)

var _ State = (*state0)(nil)

func load0(store adt.Store, root cid.Cid) (State, error) {
	out := state0{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make0(store adt.Store) (State, error) {
	out := state0{store: store}
	out.State = *cron0.ConstructState(cron0.BuiltInEntries())
	return &out, nil
}

type state0 struct {
	cron0.State
	store adt.Store
}

func (s *state0) GetState() interface{} {
	return &s.State
}
