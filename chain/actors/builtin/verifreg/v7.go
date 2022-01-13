package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
	verifreg7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/verifreg"
	adt7 "github.com/filecoin-project/specs-actors/v7/actors/util/adt"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state7{store: store}

	s, err := verifreg7.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state7 struct {
	verifreg7.State
	store adt.Store
}

func (s *state7) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state7) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version7, s.verifiedClients, addr)
}

func (s *state7) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version7, s.verifiers, addr)
}

func (s *state7) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version7, s.verifiers, cb)
}

func (s *state7) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version7, s.verifiedClients, cb)
}

func (s *state7) verifiedClients() (adt.Map, error) {
	return adt7.AsMap(s.store, s.VerifiedClients, builtin7.DefaultHamtBitwidth)
}

func (s *state7) verifiers() (adt.Map, error) {
	return adt7.AsMap(s.store, s.Verifiers, builtin7.DefaultHamtBitwidth)
}

func (s *state7) GetState() interface{} {
	return &s.State
}
