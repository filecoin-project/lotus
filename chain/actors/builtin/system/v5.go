package system

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	system5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/system"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store) (State, error) {
	out := state5{store: store}
	out.State = system5.State{}
	return &out, nil
}

type state5 struct {
	system5.State
	store adt.Store
}

func (s *state5) GetState() interface{} {
	return &s.State
}
