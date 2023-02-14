package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap12 "github.com/filecoin-project/go-state-types/builtin/v12/datacap"
	adt12 "github.com/filecoin-project/go-state-types/builtin/v12/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state12)(nil)

func load12(store adt.Store, root cid.Cid) (State, error) {
	out := state12{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make12(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state12{store: store}
	s, err := datacap12.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state12 struct {
	datacap12.State
	store adt.Store
}

func (s *state12) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state12) GetState() interface{} {
	return &s.State
}

func (s *state12) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version12, s.verifiedClients, cb)
}

func (s *state12) verifiedClients() (adt.Map, error) {
	return adt12.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state12) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version12, s.verifiedClients, addr)
}

func (s *state12) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state12) ActorVersion() actorstypes.Version {
	return actorstypes.Version12
}

func (s *state12) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
