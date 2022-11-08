package system

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	system3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/system"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state3)(nil)

func load3(store adt.Store, root cid.Cid) (State, error) {
	out := state3{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make3(store adt.Store) (State, error) {
	out := state3{store: store}
	out.State = system3.State{}
	return &out, nil
}

type state3 struct {
	system3.State
	store adt.Store
}

func (s *state3) GetState() interface{} {
	return &s.State
}

func (s *state3) GetBuiltinActors() cid.Cid {

	return cid.Undef

}

func (s *state3) SetBuiltinActors(c cid.Cid) error {

	return xerrors.New("cannot set manifest cid before v8")

}
