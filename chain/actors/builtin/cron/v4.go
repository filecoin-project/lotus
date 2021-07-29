package cron

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	cron4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/cron"
)

var _ State = (*state4)(nil)

func load4(store adt.Store, root cid.Cid) (State, error) {
	out := state4{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make4(store adt.Store) (State, error) {
	out := state4{store: store}
	out.State = *cron4.ConstructState(cron4.BuiltInEntries())
	return &out, nil
}

type state4 struct {
	cron4.State
	store adt.Store
}

func (s *state4) GetState() interface{} {
	return &s.State
}
