package evm

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	evm12 "github.com/filecoin-project/go-state-types/builtin/v12/evm"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state12)(nil)

func load12(store adt.Store, root cid.Cid) (State, error) {
	out := state12{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make12(store adt.Store, bytecode cid.Cid) (State, error) {
	out := state12{store: store}
	s, err := evm12.ConstructState(store, bytecode)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state12 struct {
	evm12.State
	store adt.Store
}

func (s *state12) Nonce() (uint64, error) {
	return s.State.Nonce, nil
}

func (s *state12) IsAlive() (bool, error) {
	return s.State.Tombstone == nil, nil
}

func (s *state12) GetState() interface{} {
	return &s.State
}

func (s *state12) GetBytecodeCID() (cid.Cid, error) {
	return s.State.Bytecode, nil
}

func (s *state12) GetBytecodeHash() ([32]byte, error) {
	return s.State.BytecodeHash, nil
}

func (s *state12) GetBytecode() ([]byte, error) {
	bc, err := s.GetBytecodeCID()
	if err != nil {
		return nil, err
	}

	var byteCode abi.CborBytesTransparent
	if err := s.store.Get(s.store.Context(), bc, &byteCode); err != nil {
		return nil, err
	}

	return byteCode, nil
}
