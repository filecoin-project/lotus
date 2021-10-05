package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	verifreg6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/verifreg"
	adt6 "github.com/filecoin-project/specs-actors/v6/actors/util/adt"
)

var _ State = (*state6)(nil)

func load6(store adt.Store, root cid.Cid) (State, error) {
	out := state6{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make6(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state6{store: store}

	s, err := verifreg6.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state6 struct {
	verifreg6.State
	store adt.Store
}

func (s *state6) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state6) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version6, s.verifiedClients, addr)
}

func (s *state6) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version6, s.verifiers, addr)
}

func (s *state6) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version6, s.verifiers, cb)
}

func (s *state6) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version6, s.verifiedClients, cb)
}

func (s *state6) verifiedClients() (adt.Map, error) {
	return adt6.AsMap(s.store, s.VerifiedClients, builtin6.DefaultHamtBitwidth)
}

func (s *state6) verifiers() (adt.Map, error) {
	return adt6.AsMap(s.store, s.Verifiers, builtin6.DefaultHamtBitwidth)
}

func (s *state6) GetState() interface{} {
	return &s.State
}
