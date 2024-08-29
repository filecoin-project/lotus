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
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
	verifreg7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/verifreg"
	adt7 "github.com/filecoin-project/specs-actors/v7/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state7{store: store}

	s, err := verifreg7.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state7 struct {
	verifreg7.State
	store adt.Store
}

func (s *state7) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state7) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version7, s.verifiedClients, addr)

}

func (s *state7) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version7, s.verifiers, addr)
}

func (s *state7) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version7, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state7) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version7, s.verifiers, cb)
}

func (s *state7) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version7, s.verifiedClients, cb)

}

func (s *state7) verifiedClients() (adt.Map, error) {

	return adt7.AsMap(s.store, s.VerifiedClients, builtin7.DefaultHamtBitwidth)

}

func (s *state7) verifiers() (adt.Map, error) {
	return adt7.AsMap(s.store, s.Verifiers, builtin7.DefaultHamtBitwidth)
}

func (s *state7) removeDataCapProposalIDs() (adt.Map, error) {
	return adt7.AsMap(s.store, s.RemoveDataCapProposalIDs, builtin7.DefaultHamtBitwidth)
}

func (s *state7) GetState() interface{} {
	return &s.State
}

func (s *state7) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v7")

}

func (s *state7) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state7) ActorVersion() actorstypes.Version {
	return actorstypes.Version7
}

func (s *state7) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
