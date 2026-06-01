package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account19 "github.com/filecoin-project/go-state-types/builtin/v19/account"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state19)(nil)

func load19(store adt.Store, root cid.Cid) (State, error) {
	out := state19{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make19(store adt.Store, addr address.Address) (State, error) {
	out := state19{store: store}
	out.State = account19.State{Address: addr}
	return &out, nil
}

type state19 struct {
	account19.State
	store adt.Store
}

func (s *state19) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state19) GetState() interface{} {
	return &s.State
}

func (s *state19) ActorKey() string {
	return manifest.AccountKey
}

func (s *state19) ActorVersion() actorstypes.Version {
	return actorstypes.Version19
}

func (s *state19) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
