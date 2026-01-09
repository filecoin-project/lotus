package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap18 "github.com/filecoin-project/go-state-types/builtin/v18/datacap"
	adt18 "github.com/filecoin-project/go-state-types/builtin/v18/util/adt"
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

func make18(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state18{store: store}
	s, err := datacap18.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state18 struct {
	datacap18.State
	store adt.Store
}

func (s *state18) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state18) GetState() interface{} {
	return &s.State
}

func (s *state18) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version18, s.verifiedClients, cb)
}

func (s *state18) verifiedClients() (adt.Map, error) {
	return adt18.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state18) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version18, s.verifiedClients, addr)
}

func (s *state18) ActorKey() string {
	return manifest.DatacapKey
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
