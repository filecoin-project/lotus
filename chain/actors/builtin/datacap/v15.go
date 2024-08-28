package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap15 "github.com/filecoin-project/go-state-types/builtin/v15/datacap"
	adt15 "github.com/filecoin-project/go-state-types/builtin/v15/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state15)(nil)

func load15(store adt.Store, root cid.Cid) (State, error) {
	out := state15{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make15(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state15{store: store}
	s, err := datacap15.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state15 struct {
	datacap15.State
	store adt.Store
}

func (s *state15) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state15) GetState() interface{} {
	return &s.State
}

func (s *state15) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version15, s.verifiedClients, cb)
}

func (s *state15) verifiedClients() (adt.Map, error) {
	return adt15.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state15) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version15, s.verifiedClients, addr)
}

func (s *state15) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state15) ActorVersion() actorstypes.Version {
	return actorstypes.Version15
}

func (s *state15) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
