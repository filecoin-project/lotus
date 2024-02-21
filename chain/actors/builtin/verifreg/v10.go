package verifreg

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	builtin10 "github.com/filecoin-project/go-state-types/builtin"
	adt10 "github.com/filecoin-project/go-state-types/builtin/v10/util/adt"
	verifreg10 "github.com/filecoin-project/go-state-types/builtin/v10/verifreg"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state10{store: store}

	s, err := verifreg10.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state10 struct {
	verifreg10.State
	store adt.Store
}

func (s *state10) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state10) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return false, big.Zero(), xerrors.Errorf("unsupported in actors v10")

}

func (s *state10) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version10, s.verifiers, addr)
}

func (s *state10) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version10, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state10) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version10, s.verifiers, cb)
}

func (s *state10) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return xerrors.Errorf("unsupported in actors v10")

}

func (s *state10) verifiedClients() (adt.Map, error) {

	return nil, xerrors.Errorf("unsupported in actors v10")

}

func (s *state10) verifiers() (adt.Map, error) {
	return adt10.AsMap(s.store, s.Verifiers, builtin10.DefaultHamtBitwidth)
}

func (s *state10) removeDataCapProposalIDs() (adt.Map, error) {
	return adt10.AsMap(s.store, s.RemoveDataCapProposalIDs, builtin10.DefaultHamtBitwidth)
}

func (s *state10) GetState() interface{} {
	return &s.State
}

func (s *state10) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	alloc, ok, err := s.FindAllocation(s.store, clientIdAddr, verifreg10.AllocationId(allocationId))
	return (*Allocation)(alloc), ok, err
}

func (s *state10) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	v10Map, err := s.LoadAllocationsToMap(s.store, clientIdAddr)

	retMap := make(map[AllocationId]Allocation, len(v10Map))
	for k, v := range v10Map {
		retMap[AllocationId(k)] = Allocation(v)
	}

	return retMap, err

}

func (s *state10) GetAllAllocations() (map[AllocationId]Allocation, error) {

	v10Map, err := s.State.GetAllAllocations(s.store)

	retMap := make(map[AllocationId]Allocation, len(v10Map))
	for k, v := range v10Map {
		retMap[AllocationId(k)] = Allocation(v)
	}

	return retMap, err

}

func (s *state10) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	claim, ok, err := s.FindClaim(s.store, providerIdAddr, verifreg10.ClaimId(claimId))
	return (*Claim)(claim), ok, err

}

func (s *state10) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	v10Map, err := s.LoadClaimsToMap(s.store, providerIdAddr)

	retMap := make(map[ClaimId]Claim, len(v10Map))
	for k, v := range v10Map {
		retMap[ClaimId(k)] = Claim(v)
	}

	return retMap, err

}

func (s *state10) GetAllClaims() (map[ClaimId]Claim, error) {

	v10Map, err := s.State.GetAllClaims(s.store)

	retMap := make(map[ClaimId]Claim, len(v10Map))
	for k, v := range v10Map {
		retMap[ClaimId(k)] = Claim(v)
	}

	return retMap, err

}

func (s *state10) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	v10Map, err := s.LoadClaimsToMap(s.store, providerIdAddr)

	retMap := make(map[abi.SectorNumber][]ClaimId)
	for k, v := range v10Map {
		claims, ok := retMap[v.Sector]
		if !ok {
			retMap[v.Sector] = []ClaimId{ClaimId(k)}
		} else {
			retMap[v.Sector] = append(claims, ClaimId(k))
		}
	}

	return retMap, err

}

func (s *state10) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state10) ActorVersion() actorstypes.Version {
	return actorstypes.Version10
}

func (s *state10) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
