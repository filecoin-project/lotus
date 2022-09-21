package datacap

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	datacap9 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"

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

func make9(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state9{store: store}
	s, err := datacap9.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state9 struct {
	datacap9.State
	store adt.Store
}

func (s *state9) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state9) GetState() interface{} {
	return &s.State
}
