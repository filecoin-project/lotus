package verifreg

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
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

	return false, big.Zero(), xerrors.Errorf("unsupported in actors v9")

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

	return xerrors.Errorf("unsupported in actors v9")

}

func (s *state9) verifiedClients() (adt.Map, error) {

	return nil, xerrors.Errorf("unsupported in actors v9")

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

func (s *state9) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*verifreg9.Allocation, bool, error) {

	return s.FindAllocation(s.store, clientIdAddr, allocationId)

}

func (s *state9) GetAllocations(clientIdAddr address.Address) (map[verifreg9.AllocationId]verifreg9.Allocation, error) {

	return s.LoadAllocationsToMap(s.store, clientIdAddr)

}

func (s *state9) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*verifreg9.Claim, bool, error) {

	return s.FindClaim(s.store, providerIdAddr, claimId)

}

func (s *state9) GetClaims(providerIdAddr address.Address) (map[verifreg9.ClaimId]verifreg9.Claim, error) {

	return s.LoadClaimsToMap(s.store, providerIdAddr)

}
