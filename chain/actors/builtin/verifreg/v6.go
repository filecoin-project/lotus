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
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	verifreg6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/verifreg"
	adt6 "github.com/filecoin-project/specs-actors/v6/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state6)(nil)

func load6(store adt.Store, root cid.Cid) (State, error) {
	out := state6{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make6(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state6{store: store}

	s, err := verifreg6.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state6 struct {
	verifreg6.State
	store adt.Store
}

func (s *state6) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state6) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version6, s.verifiedClients, addr)

}

func (s *state6) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version6, s.verifiers, addr)
}

func (s *state6) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version6, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state6) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version6, s.verifiers, cb)
}

func (s *state6) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version6, s.verifiedClients, cb)

}

func (s *state6) verifiedClients() (adt.Map, error) {

	return adt6.AsMap(s.store, s.VerifiedClients, builtin6.DefaultHamtBitwidth)

}

func (s *state6) verifiers() (adt.Map, error) {
	return adt6.AsMap(s.store, s.Verifiers, builtin6.DefaultHamtBitwidth)
}

func (s *state6) removeDataCapProposalIDs() (adt.Map, error) {
	return nil, nil

}

func (s *state6) GetState() interface{} {
	return &s.State
}

func (s *state6) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v6")

}

func (s *state6) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state6) ActorVersion() actorstypes.Version {
	return actorstypes.Version6
}

func (s *state6) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
