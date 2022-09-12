package system

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	system0 "github.com/filecoin-project/specs-actors/actors/builtin/system"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state0)(nil)

func load0(store adt.Store, root cid.Cid) (State, error) {
	out := state0{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make0(store adt.Store) (State, error) {
	out := state0{store: store}
	out.State = system0.State{}
	return &out, nil
}

type state0 struct {
	system0.State
	store adt.Store
}

func (s *state0) GetState() interface{} {
	return &s.State
}

func (s *state0) GetBuiltinActors() cid.Cid {

	return cid.Undef

}

func (s *state0) SetBuiltinActors(c cid.Cid) error {

	return xerrors.New("cannot set manifest cid before v8")

}
