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
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	verifreg3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/verifreg"
	adt3 "github.com/filecoin-project/specs-actors/v3/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state3)(nil)

func load3(store adt.Store, root cid.Cid) (State, error) {
	out := state3{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make3(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state3{store: store}

	s, err := verifreg3.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state3 struct {
	verifreg3.State
	store adt.Store
}

func (s *state3) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state3) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version3, s.verifiedClients, addr)

}

func (s *state3) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version3, s.verifiers, addr)
}

func (s *state3) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version3, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state3) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version3, s.verifiers, cb)
}

func (s *state3) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version3, s.verifiedClients, cb)

}

func (s *state3) verifiedClients() (adt.Map, error) {

	return adt3.AsMap(s.store, s.VerifiedClients, builtin3.DefaultHamtBitwidth)

}

func (s *state3) verifiers() (adt.Map, error) {
	return adt3.AsMap(s.store, s.Verifiers, builtin3.DefaultHamtBitwidth)
}

func (s *state3) removeDataCapProposalIDs() (adt.Map, error) {
	return nil, nil

}

func (s *state3) GetState() interface{} {
	return &s.State
}

func (s *state3) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v3")

}

func (s *state3) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state3) ActorVersion() actorstypes.Version {
	return actorstypes.Version3
}

func (s *state3) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
