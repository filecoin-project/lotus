package evm

import (
	"github.com/ipfs/go-cid"

	evm11 "github.com/filecoin-project/go-state-types/builtin/v11/evm"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state11)(nil)

func load11(store adt.Store, root cid.Cid) (State, error) {
	out := state11{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make11(store adt.Store, bytecode cid.Cid) (State, error) {
	out := state11{store: store}
	s, err := evm11.ConstructState(store, bytecode)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state11 struct {
	evm11.State
	store adt.Store
}

func (s *state11) Nonce() (uint64, error) {
	return s.State.Nonce, nil
}

func (s *state11) GetState() interface{} {
	return &s.State
}
