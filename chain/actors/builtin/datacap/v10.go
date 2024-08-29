package datacap

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	datacap10 "github.com/filecoin-project/go-state-types/builtin/v10/datacap"
	adt10 "github.com/filecoin-project/go-state-types/builtin/v10/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store, governor address.Address, bitwidth uint64) (State, error) {
	out := state10{store: store}
	s, err := datacap10.ConstructState(store, governor, bitwidth)
	if err != nil {
		return nil, err
	}

	out.State = *s

	return &out, nil
}

type state10 struct {
	datacap10.State
	store adt.Store
}

func (s *state10) Governor() (address.Address, error) {
	return s.State.Governor, nil
}

func (s *state10) GetState() interface{} {
	return &s.State
}

func (s *state10) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachClient(s.store, actors.Version10, s.verifiedClients, cb)
}

func (s *state10) verifiedClients() (adt.Map, error) {
	return adt10.AsMap(s.store, s.Token.Balances, int(s.Token.HamtBitWidth))
}

func (s *state10) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, actors.Version10, s.verifiedClients, addr)
}

func (s *state10) ActorKey() string {
	return manifest.DatacapKey
}

func (s *state10) ActorVersion() actorstypes.Version {
	return actorstypes.Version10
}

func (s *state10) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
