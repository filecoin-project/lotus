package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap14 "github.com/filecoin-project/go-state-types/builtin/v14/datacap"
	adt14 "github.com/filecoin-project/go-state-types/builtin/v14/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state14{store: store}
	s, err := datacap14.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state14 struct {
	datacap14.State
	store adt.Store
}

func (s *state14) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version14, s.verifiedClients, cb)
}

func (s *state14) verifiedClients() (adt.Map, error) {
	return adt14.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state14) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version14, s.verifiedClients, addr)
}

func (s *state14) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
