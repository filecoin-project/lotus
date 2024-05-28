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
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	verifreg5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/verifreg"
	adt5 "github.com/filecoin-project/specs-actors/v5/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state5{store: store}

	s, err := verifreg5.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state5 struct {
	verifreg5.State
	store adt.Store
}

func (s *state5) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state5) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version5, s.verifiedClients, addr)

}

func (s *state5) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version5, s.verifiers, addr)
}

func (s *state5) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version5, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state5) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version5, s.verifiers, cb)
}

func (s *state5) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version5, s.verifiedClients, cb)

}

func (s *state5) verifiedClients() (adt.Map, error) {

	return adt5.AsMap(s.store, s.VerifiedClients, builtin5.DefaultHamtBitwidth)

}

func (s *state5) verifiers() (adt.Map, error) {
	return adt5.AsMap(s.store, s.Verifiers, builtin5.DefaultHamtBitwidth)
}

func (s *state5) removeDataCapProposalIDs() (adt.Map, error) {
	return nil, nil

}

func (s *state5) GetState() interface{} {
	return &s.State
}

func (s *state5) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v5")

}

func (s *state5) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state5) ActorVersion() actorstypes.Version {
	return actorstypes.Version5
}

func (s *state5) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
