package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"
	account3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/account"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state3)(nil)

func load3(store adt.Store, root cid.Cid) (State, error) {
	out := state3{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make3(store adt.Store, addr address.Address) (State, error) {
	out := state3{store: store}
	out.State = account3.State{Address: addr}
	return &out, nil
}

type state3 struct {
	account3.State
	store adt.Store
}

func (s *state3) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state3) GetState() interface{} {
	return &s.State
}

func (s *state3) ActorKey() string {
	return manifest.AccountKey
}

func (s *state3) ActorVersion() actorstypes.Version {
	return actorstypes.Version3
}

func (s *state3) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
