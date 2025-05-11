package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account17 "github.com/filecoin-project/go-state-types/builtin/v17/account"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state17)(nil)

func load17(store adt.Store, root cid.Cid) (State, error) {
	out := state17{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make17(store adt.Store, addr address.Address) (State, error) {
	out := state17{store: store}
	out.State = account17.State{Address: addr}
	return &out, nil
}

type state17 struct {
	account17.State
	store adt.Store
}

func (s *state17) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state17) GetState() interface{} {
	return &s.State
}

func (s *state17) ActorKey() string {
	return manifest.AccountKey
}

func (s *state17) ActorVersion() actorstypes.Version {
	return actorstypes.Version17
}

func (s *state17) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
