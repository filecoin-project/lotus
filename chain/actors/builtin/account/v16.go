package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account16 "github.com/filecoin-project/go-state-types/builtin/v16/account"
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

func make16(store adt.Store, addr address.Address) (State, error) {
	out := state16{store: store}
	out.State = account16.State{Address: addr}
	return &out, nil
}

type state16 struct {
	account16.State
	store adt.Store
}

func (s *state16) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state16) GetState() interface{} {
	return &s.State
}

func (s *state16) ActorKey() string {
	return manifest.AccountKey
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
