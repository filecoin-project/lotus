package evm

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	evm14 "github.com/filecoin-project/go-state-types/builtin/v14/evm"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store, bytecode cid.Cid) (State, error) {
	out := state14{store: store}
	s, err := evm14.ConstructState(store, bytecode)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state14 struct {
	evm14.State
	store adt.Store
}

func (s *state14) Nonce() (uint64, error) {
	return s.State.Nonce, nil
}

func (s *state14) IsAlive() (bool, error) {
	return s.State.Tombstone == nil, nil
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) GetBytecodeCID() (cid.Cid, error) {
	return s.State.Bytecode, nil
}

func (s *state14) GetBytecodeHash() ([32]byte, error) {
	return s.State.BytecodeHash, nil
}

func (s *state14) GetBytecode() ([]byte, error) {
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
