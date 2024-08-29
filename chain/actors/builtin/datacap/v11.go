package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap11 "github.com/filecoin-project/go-state-types/builtin/v11/datacap"
	adt11 "github.com/filecoin-project/go-state-types/builtin/v11/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state11)(nil)

func load11(store adt.Store, root cid.Cid) (State, error) {
	out := state11{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make11(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state11{store: store}
	s, err := datacap11.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state11 struct {
	datacap11.State
	store adt.Store
}

func (s *state11) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state11) GetState() interface{} {
	return &s.State
}

func (s *state11) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version11, s.verifiedClients, cb)
}

func (s *state11) verifiedClients() (adt.Map, error) {
	return adt11.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state11) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version11, s.verifiedClients, addr)
}

func (s *state11) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state11) ActorVersion() actorstypes.Version {
	return actorstypes.Version11
}

func (s *state11) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
