package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
)

var _ State = (*state0)(nil)

type state0 struct {
	verifreg0.State
	store adt.Store
}

func (s *state0) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, builtin.Version0, s.State.VerifiedClients, addr)
}

func (s *state0) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, builtin.Version0, s.State.Verifiers, addr)
}

func (s *state0) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, builtin.Version0, s.State.Verifiers, cb)
}

func (s *state0) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, builtin.Version0, s.State.VerifiedClients, cb)
}
