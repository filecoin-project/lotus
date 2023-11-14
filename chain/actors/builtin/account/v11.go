package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account11 "github.com/filecoin-project/go-state-types/builtin/v11/account"
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

func make11(store adt.Store, addr address.Address) (State, error) {
	out := state11{store: store}
	out.State = account11.State{Address: addr}
	return &out, nil
}

type state11 struct {
	account11.State
	store adt.Store
}

func (s *state11) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state11) GetState() interface{} {
	return &s.State
}

func (s *state11) ActorKey() string {
	return manifest.AccountKey
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
