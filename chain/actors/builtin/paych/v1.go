package paych

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	paych1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/paych"
	adt1 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
)

var _ State = (*state1)(nil)

func load1(store adt.Store, root cid.Cid) (State, error) {
	out := state1{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type state1 struct {
	paych1.State
	store adt.Store
	lsAmt *adt1.Array
}

// Channel owner, who has funded the actor
func (s *state1) From() (address.Address, error) {
	return s.State.From, nil
}

// Recipient of payouts from channel
func (s *state1) To() (address.Address, error) {
	return s.State.To, nil
}

// Height at which the channel can be `Collected`
func (s *state1) SettlingAt() (abi.ChainEpoch, error) {
	return s.State.SettlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state1) ToSend() (abi.TokenAmount, error) {
	return s.State.ToSend, nil
}

func (s *state1) getOrLoadLsAmt() (*adt1.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt1.AsArray(s.store, s.State.LaneStates)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state1) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

// Iterate lane states
func (s *state1) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych1.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState1{ls})
	})
}

type laneState1 struct {
	paych1.LaneState
}

func (ls *laneState1) Redeemed() (big.Int, error) {
	return ls.LaneState.Redeemed, nil
}

func (ls *laneState1) Nonce() (uint64, error) {
	return ls.LaneState.Nonce, nil
}
