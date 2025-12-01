package evm

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	evm18 "github.com/filecoin-project/go-state-types/builtin/v18/evm"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state18)(nil)

func load18(store adt.Store, root cid.Cid) (State, error) {
	out := state18{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make18(store adt.Store, bytecode cid.Cid) (State, error) {
	out := state18{store: store}
	s, err := evm18.ConstructState(store, bytecode)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state18 struct {
	evm18.State
	store adt.Store
}

func (s *state18) Nonce() (uint64, error) {
	return s.State.Nonce, nil
}

func (s *state18) IsAlive() (bool, error) {
	return s.State.Tombstone == nil, nil
}

func (s *state18) GetState() interface{} {
	return &s.State
}

func (s *state18) GetBytecodeCID() (cid.Cid, error) {
	return s.State.Bytecode, nil
}

func (s *state18) GetBytecodeHash() ([32]byte, error) {
	return s.State.BytecodeHash, nil
}

func (s *state18) GetBytecode() ([]byte, error) {
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
