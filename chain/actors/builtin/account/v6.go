package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"
	account6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/account"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state6)(nil)

func load6(store adt.Store, root cid.Cid) (State, error) {
	out := state6{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make6(store adt.Store, addr address.Address) (State, error) {
	out := state6{store: store}
	out.State = account6.State{Address: addr}
	return &out, nil
}

type state6 struct {
	account6.State
	store adt.Store
}

func (s *state6) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state6) GetState() interface{} {
	return &s.State
}

func (s *state6) ActorKey() string {
	return manifest.AccountKey
}

func (s *state6) ActorVersion() actorstypes.Version {
	return actorstypes.Version6
}

func (s *state6) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
