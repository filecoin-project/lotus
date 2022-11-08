package cron

import (
	"github.com/ipfs/go-cid"

	cron9 "github.com/filecoin-project/go-state-types/builtin/v9/cron"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store) (State, error) {
	out := state9{store: store}
	out.State = *cron9.ConstructState(cron9.BuiltInEntries())
	return &out, nil
}

type state9 struct {
	cron9.State
	store adt.Store
}

func (s *state9) GetState() interface{} {
	return &s.State
}
