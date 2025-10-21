package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account18 "github.com/filecoin-project/go-state-types/builtin/v18/account"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state18)(nil)

func load18(store adt.Store, root cid.Cid) (State, error) {
	out := state18{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make18(store adt.Store, addr address.Address) (State, error) {
	out := state18{store: store}
	out.State = account18.State{Address: addr}
	return &out, nil
}

type state18 struct {
	account18.State
	store adt.Store
}

func (s *state18) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state18) GetState() interface{} {
	return &s.State
}

func (s *state18) ActorKey() string {
	return manifest.AccountKey
}

func (s *state18) ActorVersion() actorstypes.Version {
	return actorstypes.Version18
}

func (s *state18) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
