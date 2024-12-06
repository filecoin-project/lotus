package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap16 "github.com/filecoin-project/go-state-types/builtin/v16/datacap"
	adt16 "github.com/filecoin-project/go-state-types/builtin/v16/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state16)(nil)

func load16(store adt.Store, root cid.Cid) (State, error) {
	out := state16{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make16(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state16{store: store}
	s, err := datacap16.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state16 struct {
	datacap16.State
	store adt.Store
}

func (s *state16) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state16) GetState() interface{} {
	return &s.State
}

func (s *state16) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version16, s.verifiedClients, cb)
}

func (s *state16) verifiedClients() (adt.Map, error) {
	return adt16.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state16) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version16, s.verifiedClients, addr)
}

func (s *state16) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state16) ActorVersion() actorstypes.Version {
	return actorstypes.Version16
}

func (s *state16) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
