package verifreg

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtin9 "github.com/filecoin-project/go-state-types/builtin"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state9{store: store}

	s, err := verifreg9.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state9 struct {
	verifreg9.State
	store adt.Store
}

func (s *state9) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state9) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version9, s.verifiedClients, addr)
}

func (s *state9) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version9, s.verifiers, addr)
}

func (s *state9) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version9, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state9) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version9, s.verifiers, cb)
}

func (s *state9) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version9, s.verifiedClients, cb)
}

func (s *state9) verifiedClients() (adt.Map, error) {
	return adt9.AsMap(s.store, s.VerifiedClients, builtin9.DefaultHamtBitwidth)
}

func (s *state9) verifiers() (adt.Map, error) {
	return adt9.AsMap(s.store, s.Verifiers, builtin9.DefaultHamtBitwidth)
}

func (s *state9) removeDataCapProposalIDs() (adt.Map, error) {
	return adt9.AsMap(s.store, s.RemoveDataCapProposalIDs, builtin9.DefaultHamtBitwidth)
}

func (s *state9) GetState() interface{} {
	return &s.State
}
