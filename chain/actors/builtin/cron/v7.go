package cron

import (
	"fmt"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"
	cron7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/cron"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state7)(nil)

func load7(store adt.Store, root cid.Cid) (State, error) {
	out := state7{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make7(store adt.Store) (State, error) {
	out := state7{store: store}
	out.State = *cron7.ConstructState(cron7.BuiltInEntries())
	return &out, nil
}

type state7 struct {
	cron7.State
	store adt.Store
}

func (s *state7) GetState() interface{} {
	return &s.State
}

func (s *state7) ActorKey() string {
	return manifest.CronKey
}

func (s *state7) ActorVersion() actorstypes.Version {
	return actorstypes.Version7
}

func (s *state7) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
