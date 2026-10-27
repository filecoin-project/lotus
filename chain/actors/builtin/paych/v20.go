package paych

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	paych20 "github.com/filecoin-project/go-state-types/builtin/v20/paych"
	adt20 "github.com/filecoin-project/go-state-types/builtin/v20/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state20)(nil)

func load20(store adt.Store, root cid.Cid) (State, error) {
	out := state20{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make20(store adt.Store) (State, error) {
	out := state20{store: store}
	out.State = paych20.State{}
	return &out, nil
}

type state20 struct {
	paych20.State
	store adt.Store
	lsAmt *adt20.Array
}

// Channel owner, who has funded the actor
func (s *state20) From() (address.Address, error) {
	return s.State.From, nil
}

// Recipient of payouts from channel
func (s *state20) To() (address.Address, error) {
	return s.State.To, nil
}

// Height at which the channel can be `Collected`
func (s *state20) SettlingAt() (abi.ChainEpoch, error) {
	return s.State.SettlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state20) ToSend() (abi.TokenAmount, error) {
	return s.State.ToSend, nil
}

func (s *state20) getOrLoadLsAmt() (*adt20.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt20.AsArray(s.store, s.State.LaneStates, paych20.LaneStatesAmtBitwidth)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state20) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

func (s *state20) GetState() interface{} {
	return &s.State
}

// Iterate lane states
func (s *state20) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych20.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState20{ls})
	})
}

type laneState20 struct {
	paych20.LaneState
}

func (ls *laneState20) Redeemed() (big.Int, error) {
	return ls.LaneState.Redeemed, nil
}

func (ls *laneState20) Nonce() (uint64, error) {
	return ls.LaneState.Nonce, nil
}

func (s *state20) ActorKey() string {
	return manifest.PaychKey
}

func (s *state20) ActorVersion() actorstypes.Version {
	return actorstypes.Version20
}

func (s *state20) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
