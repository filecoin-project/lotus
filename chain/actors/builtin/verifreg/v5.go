package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	verifreg5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/verifreg"
	adt5 "github.com/filecoin-project/specs-actors/v5/actors/util/adt"
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

func make5(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state5{store: store}

	s, err := verifreg5.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state5 struct {
	verifreg5.State
	store adt.Store
}

func (s *state5) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state5) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version5, s.verifiedClients, addr)
}

func (s *state5) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version5, s.verifiers, addr)
}

func (s *state5) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version5, s.verifiers, cb)
}

func (s *state5) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version5, s.verifiedClients, cb)
}

func (s *state5) verifiedClients() (adt.Map, error) {
	return adt5.AsMap(s.store, s.VerifiedClients, builtin5.DefaultHamtBitwidth)
}

func (s *state5) verifiers() (adt.Map, error) {
	return adt5.AsMap(s.store, s.Verifiers, builtin5.DefaultHamtBitwidth)
}

func (s *state5) GetState() interface{} {
	return &s.State
}
