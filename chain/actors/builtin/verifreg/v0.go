package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
)

var _ State = (*state0)(nil)

func load0(store adt.Store, root cid.Cid) (State, error) {
	out := state0{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make0(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state0{store: store}

	em, err := adt0.MakeEmptyMap(store).Root()
	if err != nil {
		return nil, err
	}

	out.State = *verifreg0.ConstructState(em, rootKeyAddress)

	return &out, nil
}

type state0 struct {
	verifreg0.State
	store adt.Store
}

func (s *state0) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state0) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version0, s.verifiedClients, addr)
}

func (s *state0) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version0, s.verifiers, addr)
}

func (s *state0) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version0, s.verifiers, cb)
}

func (s *state0) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version0, s.verifiedClients, cb)
}

func (s *state0) verifiedClients() (adt.Map, error) {
	return adt0.AsMap(s.store, s.VerifiedClients)
}

func (s *state0) verifiers() (adt.Map, error) {
	return adt0.AsMap(s.store, s.Verifiers)
}

func (s *state0) GetState() interface{} {
	return &s.State
}
