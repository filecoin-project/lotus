package paych

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	big "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	v0adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type v0State struct {
	paych.State
	store adt.Store
	lsAmt *v0adt.Array
}

// Channel owner, who has funded the actor
func (s *v0State) From() address.Address {
	return s.State.From
}

// Recipient of payouts from channel
func (s *v0State) To() address.Address {
	return s.State.To
}

// Height at which the channel can be `Collected`
func (s *v0State) SettlingAt() abi.ChainEpoch {
	return s.State.SettlingAt
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *v0State) ToSend() abi.TokenAmount {
	return s.State.ToSend
}

func (s *v0State) getOrLoadLsAmt() (*v0adt.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := v0adt.AsArray(s.store, s.State.LaneStates)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *v0State) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

// Iterate lane states
func (s *v0State) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
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
		return cb(uint64(i), &v0LaneState{ls})
	})
}

type v0LaneState struct {
	paych.LaneState
}

func (ls *v0LaneState) Redeemed() big.Int {
	return ls.LaneState.Redeemed
}

func (ls *v0LaneState) Nonce() uint64 {
	return ls.LaneState.Nonce
}
