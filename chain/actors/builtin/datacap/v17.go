package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap17 "github.com/filecoin-project/go-state-types/builtin/v17/datacap"
	adt17 "github.com/filecoin-project/go-state-types/builtin/v17/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state17)(nil)

func load17(store adt.Store, root cid.Cid) (State, error) {
	out := state17{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make17(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state17{store: store}
	s, err := datacap17.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state17 struct {
	datacap17.State
	store adt.Store
}

func (s *state17) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state17) GetState() interface{} {
	return &s.State
}

func (s *state17) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version17, s.verifiedClients, cb)
}

func (s *state17) verifiedClients() (adt.Map, error) {
	return adt17.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state17) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version17, s.verifiedClients, addr)
}

func (s *state17) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state17) ActorVersion() actorstypes.Version {
	return actorstypes.Version17
}

func (s *state17) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
