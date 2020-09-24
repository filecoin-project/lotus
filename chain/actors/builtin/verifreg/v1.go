package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	verifreg1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
)

var _ State = (*state1)(nil)

func load1(store adt.Store, root cid.Cid) (State, error) {
	out := state1{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type state1 struct {
	verifreg1.State
	store adt.Store
}

func (s *state1) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version1, s.State.VerifiedClients, addr)
}

func (s *state1) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version1, s.State.Verifiers, addr)
}

func (s *state1) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version1, s.State.Verifiers, cb)
}

func (s *state1) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version1, s.State.VerifiedClients, cb)
}
