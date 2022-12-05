package evm

import (
	"github.com/ipfs/go-cid"

	evm10 "github.com/filecoin-project/go-state-types/builtin/v10/evm"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store, bytecode cid.Cid) (State, error) {
	out := state10{store: store}
	s, err := evm10.ConstructState(store, bytecode)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state10 struct {
	evm10.State
	store adt.Store
}

func (s *state10) Nonce() (uint64, error) {
	return s.State.Nonce, nil
}

func (s *state10) GetState() interface{} {
	return &s.State
}
