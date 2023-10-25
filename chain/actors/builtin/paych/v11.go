package paych

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	paych11 "github.com/filecoin-project/go-state-types/builtin/v11/paych"
	adt11 "github.com/filecoin-project/go-state-types/builtin/v11/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state11)(nil)

func load11(store adt.Store, root cid.Cid) (State, error) {
	out := state11{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make11(store adt.Store) (State, error) {
	out := state11{store: store}
	out.State = paych11.State{}
	return &out, nil
}

type state11 struct {
	paych11.State
	store adt.Store
	lsAmt *adt11.Array
}

// Channel owner, who has funded the actor
func (s *state11) From() (address.Address, error) {
	return s.State.From, nil
}

// Recipient of payouts from channel
func (s *state11) To() (address.Address, error) {
	return s.State.To, nil
}

// Height at which the channel can be `Collected`
func (s *state11) SettlingAt() (abi.ChainEpoch, error) {
	return s.State.SettlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state11) ToSend() (abi.TokenAmount, error) {
	return s.State.ToSend, nil
}

func (s *state11) getOrLoadLsAmt() (*adt11.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt11.AsArray(s.store, s.State.LaneStates, paych11.LaneStatesAmtBitwidth)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state11) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

func (s *state11) GetState() interface{} {
	return &s.State
}

// Iterate lane states
func (s *state11) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych11.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState11{ls})
	})
}

type laneState11 struct {
	paych11.LaneState
}

func (ls *laneState11) Redeemed() (big.Int, error) {
	return ls.LaneState.Redeemed, nil
}

func (ls *laneState11) Nonce() (uint64, error) {
	return ls.LaneState.Nonce, nil
}

func (s *state11) ActorKey() string {
	return manifest.PaychKey
}

func (s *state11) ActorVersion() actorstypes.Version {
	return actorstypes.Version11
}

func (s *state11) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
