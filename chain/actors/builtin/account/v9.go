package account

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	account9 "github.com/filecoin-project/go-state-types/builtin/v9/account"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store, addr address.Address) (State, error) {
	out := state9{store: store}
	out.State = account9.State{Address: addr}
	return &out, nil
}

type state9 struct {
	account9.State
	store adt.Store
}

func (s *state9) PubkeyAddress() (address.Address, error) {
	return s.Address, nil
}

func (s *state9) GetState() interface{} {
	return &s.State
}
