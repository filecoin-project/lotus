package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	account8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/account"
)

var _ State = (*state8)(nil)

func load8(store adt.Store, root cid.Cid) (State, error) {
	out := state8{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make8(store adt.Store, addr address.Address) (State, error) {
	out := state8{store: store}
	out.State = account8.State{Address: addr}
	return &out, nil
}

type state8 struct {
	account8.State
	store adt.Store
}

func (s *state8) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state8) GetState() interface{} {
	return &s.State
}
