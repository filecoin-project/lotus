package paych

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"
	paych7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/paych"
	adt7 "github.com/filecoin-project/specs-actors/v7/actors/util/adt"

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
	out.State = paych7.State{}
	return &out, nil
}

type state7 struct {
	paych7.State
	store adt.Store
	lsAmt *adt7.Array
}

// Channel owner, who has funded the actor
func (s *state7) From() (address.Address, error) {
	return s.State.From, nil
}

// Recipient of payouts from channel
func (s *state7) To() (address.Address, error) {
	return s.State.To, nil
}

// Height at which the channel can be `Collected`
func (s *state7) SettlingAt() (abi.ChainEpoch, error) {
	return s.State.SettlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state7) ToSend() (abi.TokenAmount, error) {
	return s.State.ToSend, nil
}

func (s *state7) getOrLoadLsAmt() (*adt7.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt7.AsArray(s.store, s.State.LaneStates, paych7.LaneStatesAmtBitwidth)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state7) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

func (s *state7) GetState() interface{} {
	return &s.State
}

// Iterate lane states
func (s *state7) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych7.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState7{ls})
	})
}

type laneState7 struct {
	paych7.LaneState
}

func (ls *laneState7) Redeemed() (big.Int, error) {
	return ls.LaneState.Redeemed, nil
}

func (ls *laneState7) Nonce() (uint64, error) {
	return ls.LaneState.Nonce, nil
}

func (s *state7) ActorKey() string {
	return manifest.PaychKey
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
