package account

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	account14 "github.com/filecoin-project/go-state-types/builtin/v14/account"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store, addr address.Address) (State, error) {
	out := state14{store: store}
	out.State = account14.State{Address: addr}
	return &out, nil
}

type state14 struct {
	account14.State
	store adt.Store
}

func (s *state14) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state14) GetState() interface{} {
	return &s.State
}

func (s *state14) ActorKey() string {
	return manifest.AccountKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
