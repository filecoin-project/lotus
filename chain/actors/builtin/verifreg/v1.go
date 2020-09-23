package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"

	verifreg1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
)

var _ State = (*state1)(nil)

type state1 struct {
	verifreg1.State
	store adt.Store
}

func (s *state1) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, builtin.Version1, s.State.VerifiedClients, addr)
}

func (s *state1) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, builtin.Version1, s.State.Verifiers, addr)
}

func (s *state1) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, builtin.Version1, s.State.Verifiers, cb)
}

func (s *state1) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, builtin.Version1, s.State.VerifiedClients, cb)
}
