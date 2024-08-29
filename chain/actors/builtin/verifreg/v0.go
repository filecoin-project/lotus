package verifreg

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/manifest"
	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
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

func (s *state0) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version0, s.removeDataCapProposalIDs, verifier, client)
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

func (s *state0) removeDataCapProposalIDs() (adt.Map, error) {
	return nil, nil

}

func (s *state0) GetState() interface{} {
	return &s.State
}

func (s *state0) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v0")

}

func (s *state0) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state0) ActorVersion() actorstypes.Version {
	return actorstypes.Version0
}

func (s *state0) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
