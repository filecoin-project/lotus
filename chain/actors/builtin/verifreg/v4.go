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
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	verifreg4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/verifreg"
	adt4 "github.com/filecoin-project/specs-actors/v4/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state4)(nil)

func load4(store adt.Store, root cid.Cid) (State, error) {
	out := state4{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make4(store adt.Store, rootKeyAddress address.Address) (State, error) {
	out := state4{store: store}

	s, err := verifreg4.ConstructState(store, rootKeyAddress)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state4 struct {
	verifreg4.State
	store adt.Store
}

func (s *state4) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state4) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {

	return getDataCap(s.store, actors.Version4, s.verifiedClients, addr)

}

func (s *state4) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version4, s.verifiers, addr)
}

func (s *state4) RemoveDataCapProposalID(verifier address.Address, client address.Address) (bool, uint64, error) {
	return getRemoveDataCapProposalID(s.store, actors.Version4, s.removeDataCapProposalIDs, verifier, client)
}

func (s *state4) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, actors.Version4, s.verifiers, cb)
}

func (s *state4) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {

	return forEachCap(s.store, actors.Version4, s.verifiedClients, cb)

}

func (s *state4) verifiedClients() (adt.Map, error) {

	return adt4.AsMap(s.store, s.VerifiedClients, builtin4.DefaultHamtBitwidth)

}

func (s *state4) verifiers() (adt.Map, error) {
	return adt4.AsMap(s.store, s.Verifiers, builtin4.DefaultHamtBitwidth)
}

func (s *state4) removeDataCapProposalIDs() (adt.Map, error) {
	return nil, nil

}

func (s *state4) GetState() interface{} {
	return &s.State
}

func (s *state4) GetAllocation(clientIdAddr address.Address, allocationId verifreg9.AllocationId) (*Allocation, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) GetAllocations(clientIdAddr address.Address) (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) GetAllAllocations() (map[AllocationId]Allocation, error) {

	return nil, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) GetClaim(providerIdAddr address.Address, claimId verifreg9.ClaimId) (*Claim, bool, error) {

	return nil, false, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) GetClaims(providerIdAddr address.Address) (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) GetAllClaims() (map[ClaimId]Claim, error) {

	return nil, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) GetClaimIdsBySector(providerIdAddr address.Address) (map[abi.SectorNumber][]ClaimId, error) {

	return nil, xerrors.Errorf("unsupported in actors v4")

}

func (s *state4) ActorKey() string {
	return manifest.VerifregKey
}

func (s *state4) ActorVersion() actorstypes.Version {
	return actorstypes.Version4
}

func (s *state4) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
