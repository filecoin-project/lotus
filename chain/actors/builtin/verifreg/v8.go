package verifreg

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin8 "github.com/filecoin-project/go-state-types/builtin"
	adt8 "github.com/filecoin-project/go-state-types/builtin/v8/util/adt"
	verifreg8 "github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state8)(nil)

func load8(store adt.Store, root cid.Cid) (State, error) {
	out := state8{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make8(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state8{store: store}

	s, err := verifreg8.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state8 struct {
	verifreg8.State
	store adt.Store
}

func (s *state8) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state8) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version8, s.verifiedClients, addr)

}

func (s *state8) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version8, s.verifiers, addr)
}

func (s *state8) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version8, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state8) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version8, s.verifiers, cb)
}

func (s *state8) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version8, s.verifiedClients, cb)

}

func (s *state8) verifiedClients() (adt.Map, error) {

	return adt8.AsMap(s.store, s.VerifiedClients, builtin8.DefaultHamtBitwidth)

}

func (s *state8) verifiers() (adt.Map, error) {
	return adt8.AsMap(s.store, s.Verifiers, builtin8.DefaultHamtBitwidth)
}

func (s *state8) removeDataCapProposalIDs() (adt.Map, error) {
	return adt8.AsMap(s.store, s.RemoveDataCapProposalIDs, builtin8.DefaultHamtBitwidth)
}

func (s *state8) GetState() interface{} {
	return &s.State
}

func (s *state8) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v8")

}

func (s *state8) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state8) ActorVersion() actorstypes.Version {
	return actorstypes.Version8
}

func (s *state8) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
