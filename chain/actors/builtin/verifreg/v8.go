package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"

	builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"
	verifreg8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/verifreg"
	adt8 "github.com/filecoin-project/specs-actors/v8/actors/util/adt"
)

var _ State = (*state8)(nil)

func load8(store adt.Store, root cid.Cid) (State, error) {
	out := state8{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make8(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state8{store: store}

	s, err := verifreg8.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state8 struct {
	verifreg8.State
	store adt.Store
}

func (s *state8) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state8) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version8, s.verifiedClients, addr)
}

func (s *state8) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version8, s.verifiers, addr)
}

func (s *state8) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version8, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state8) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version8, s.verifiers, cb)
}

func (s *state8) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version8, s.verifiedClients, cb)
}

func (s *state8) verifiedClients() (adt.Map, error) {
	return adt8.AsMap(s.store, s.VerifiedClients, builtin8.DefaultHamtBitwidth)
}

func (s *state8) verifiers() (adt.Map, error) {
	return adt8.AsMap(s.store, s.Verifiers, builtin8.DefaultHamtBitwidth)
}

func (s *state8) removeDataCapProposalIDs() (adt.Map, error) {
	return adt8.AsMap(s.store, s.RemoveDataCapProposalIDs, builtin8.DefaultHamtBitwidth)
}

func (s *state8) GetState() interface{} {
	return &s.State
}
