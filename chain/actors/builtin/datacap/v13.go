package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap13 "github.com/filecoin-project/go-state-types/builtin/v13/datacap"
	adt13 "github.com/filecoin-project/go-state-types/builtin/v13/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state13)(nil)

func load13(store adt.Store, root cid.Cid) (State, error) {
	out := state13{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make13(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state13{store: store}
	s, err := datacap13.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state13 struct {
	datacap13.State
	store adt.Store
}

func (s *state13) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state13) GetState() interface{} {
	return &s.State
}

func (s *state13) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version13, s.verifiedClients, cb)
}

func (s *state13) verifiedClients() (adt.Map, error) {
	return adt13.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state13) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version13, s.verifiedClients, addr)
}

func (s *state13) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state13) ActorVersion() actorstypes.Version {
	return actorstypes.Version13
}

func (s *state13) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
