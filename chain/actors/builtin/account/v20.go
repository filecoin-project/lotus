package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account20 "github.com/filecoin-project/go-state-types/builtin/v20/account"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state20)(nil)

func load20(store adt.Store, root cid.Cid) (State, error) {
	out := state20{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make20(store adt.Store, addr address.Address) (State, error) {
	out := state20{store: store}
	out.State = account20.State{Address: addr}
	return &out, nil
}

type state20 struct {
	account20.State
	store adt.Store
}

func (s *state20) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state20) GetState() interface{} {
	return &s.State
}

func (s *state20) ActorKey() string {
	return manifest.AccountKey
}

func (s *state20) ActorVersion() actorstypes.Version {
	return actorstypes.Version20
}

func (s *state20) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
