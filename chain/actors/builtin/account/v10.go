package account

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	account10 "github.com/filecoin-project/go-state-types/builtin/v10/account"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store, addr address.Address) (State, error) {
	out := state10{store: store}
	out.State = account10.State{Address: addr}
	return &out, nil
}

type state10 struct {
	account10.State
	store adt.Store
}

func (s *state10) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state10) GetState() interface{} {
	return &s.State
}
