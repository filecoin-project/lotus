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
	verifreg2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state2)(nil)

func load2(store adt.Store, root cid.Cid) (State, error) {
	out := state2{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make2(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state2{store: store}

	em, err := adt2.MakeEmptyMap(store).Root()
	if err != nil {
		return nil, err
	}

	out.State = *verifreg2.ConstructState(em, rootKeyAddress)

	return &out, nil
}

type state2 struct {
	verifreg2.State
	store adt.Store
}

func (s *state2) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state2) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version2, s.verifiedClients, addr)

}

func (s *state2) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version2, s.verifiers, addr)
}

func (s *state2) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version2, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state2) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version2, s.verifiers, cb)
}

func (s *state2) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version2, s.verifiedClients, cb)

}

func (s *state2) verifiedClients() (adt.Map, error) {

	return adt2.AsMap(s.store, s.VerifiedClients)

}

func (s *state2) verifiers() (adt.Map, error) {
	return adt2.AsMap(s.store, s.Verifiers)
}

func (s *state2) removeDataCapProposalIDs() (adt.Map, error) {
	return nil, nil

}

func (s *state2) GetState() interface{} {
	return &s.State
}

func (s *state2) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v2")

}

func (s *state2) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state2) ActorVersion() actorstypes.Version {
	return actorstypes.Version2
}

func (s *state2) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
