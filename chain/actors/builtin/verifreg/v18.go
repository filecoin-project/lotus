package verifreg

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	adt18 "github.com/filecoin-project/go-state-types/builtin/v18/util/adt"
	verifreg18 "github.com/filecoin-project/go-state-types/builtin/v18/verifreg"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state18)(nil)

func load18(store adt.Store, root cid.Cid) (State, error) {
	out := state18{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make18(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state18{store: store}

	s, err := verifreg18.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state18 struct {
	verifreg18.State
	store adt.Store
}

func (s *state18) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state18) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return false, big.Zero(), xerrors.Errorf("unsupported in actors v18")

}

func (s *state18) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version18, s.verifiers, addr)
}

func (s *state18) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version18, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state18) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version18, s.verifiers, cb)
}

func (s *state18) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return xerrors.Errorf("unsupported in actors v18")

}

func (s *state18) verifiedClients() (adt.Map, error) {

	return nil, xerrors.Errorf("unsupported in actors v18")

}

func (s *state18) verifiers() (adt.Map, error) {
	return adt18.AsMap(s.store, s.Verifiers, builtin18.DefaultHamtBitwidth)
}

func (s *state18) removeDataCapProposalIDs() (adt.Map, error) {
	return adt18.AsMap(s.store, s.RemoveDataCapProposalIDs, builtin18.DefaultHamtBitwidth)
}

func (s *state18) GetState() interface{} {
	return &s.State
}

func (s *state18) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	alloc, ok, err := s.FindAllocation(s.store, clientIdAddr, verifreg18.AllocationId(allocationId))
	return (*Allocation)(alloc), ok, err
}

func (s *state18) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	v18Map, err := s.LoadAllocationsToMap(s.store, clientIdAddr)

	retMap := make(map[AllocationId]Allocation, len(v18Map))
	for k, v := range v18Map {
		retMap[AllocationId(k)] = Allocation(v)
	}

	return retMap, err

}

func (s *state18) GetAllAllocations() (map[AllocationId]Allocation, error) {

	v18Map, err := s.State.GetAllAllocations(s.store)

	retMap := make(map[AllocationId]Allocation, len(v18Map))
	for k, v := range v18Map {
		retMap[AllocationId(k)] = Allocation(v)
	}

	return retMap, err

}

func (s *state18) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	claim, ok, err := s.FindClaim(s.store, providerIdAddr, verifreg18.ClaimId(claimId))
	return (*Claim)(claim), ok, err

}

func (s *state18) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	v18Map, err := s.LoadClaimsToMap(s.store, providerIdAddr)

	retMap := make(map[ClaimId]Claim, len(v18Map))
	for k, v := range v18Map {
		retMap[ClaimId(k)] = Claim(v)
	}

	return retMap, err

}

func (s *state18) GetAllClaims() (map[ClaimId]Claim, error) {

	v18Map, err := s.State.GetAllClaims(s.store)

	retMap := make(map[ClaimId]Claim, len(v18Map))
	for k, v := range v18Map {
		retMap[ClaimId(k)] = Claim(v)
	}

	return retMap, err

}

func (s *state18) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	v18Map, err := s.LoadClaimsToMap(s.store, providerIdAddr)

	retMap := make(map[abi.SectorNumber][]ClaimId)
	for k, v := range v18Map {
		claims, ok := retMap[v.Sector]
		if !ok {
			retMap[v.Sector] = []ClaimId{ClaimId(k)}
		} else {
			retMap[v.Sector] = append(claims, ClaimId(k))
		}
	}

	return retMap, err

}

func (s *state18) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state18) ActorVersion() actorstypes.Version {
	return actorstypes.Version18
}

func (s *state18) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
