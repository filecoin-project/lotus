package paych

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	big "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type state0 struct {
	paych.State
	store adt.Store
	lsAmt *adt0.Array
}

// Channel owner, who has funded the actor
func (s *state0) From() address.Address {
	return s.State.From
}

// Recipient of payouts from channel
func (s *state0) To() address.Address {
	return s.State.To
}

// Height at which the channel can be `Collected`
func (s *state0) SettlingAt() abi.ChainEpoch {
	return s.State.SettlingAt
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state0) ToSend() abi.TokenAmount {
	return s.State.ToSend
}

func (s *state0) getOrLoadLsAmt() (*adt0.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt0.AsArray(s.store, s.State.LaneStates)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state0) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

// Iterate lane states
func (s *state0) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState0{ls})
	})
}

type laneState0 struct {
	paych.LaneState
}

func (ls *laneState0) Redeemed() big.Int {
	return ls.LaneState.Redeemed
}

func (ls *laneState0) Nonce() uint64 {
	return ls.LaneState.Nonce
}
