package paych

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	paych14 "github.com/filecoin-project/go-state-types/builtin/v14/paych"
	adt14 "github.com/filecoin-project/go-state-types/builtin/v14/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state14)(nil)

func load14(store adt.Store, root cid.Cid) (State, error) {
	out := state14{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make14(store adt.Store) (State, error) {
	out := state14{store: store}
	out.State = paych14.State{}
	return &out, nil
}

type state14 struct {
	paych14.State
	store adt.Store
	lsAmt *adt14.Array
}

// Channel owner, who has funded the actor
func (s *state14) From() (address.Address, error) {
	return s.State.From, nil
}

// Recipient of payouts from channel
func (s *state14) To() (address.Address, error) {
	return s.State.To, nil
}

// Height at which the channel can be `Collected`
func (s *state14) SettlingAt() (abi.ChainEpoch, error) {
	return s.State.SettlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state14) ToSend() (abi.TokenAmount, error) {
	return s.State.ToSend, nil
}

func (s *state14) getOrLoadLsAmt() (*adt14.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt14.AsArray(s.store, s.State.LaneStates, paych14.LaneStatesAmtBitwidth)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state14) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

func (s *state14) GetState() interface{} {
	return &s.State
}

// Iterate lane states
func (s *state14) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych14.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState14{ls})
	})
}

type laneState14 struct {
	paych14.LaneState
}

func (ls *laneState14) Redeemed() (big.Int, error) {
	return ls.LaneState.Redeemed, nil
}

func (ls *laneState14) Nonce() (uint64, error) {
	return ls.LaneState.Nonce, nil
}

func (s *state14) ActorKey() string {
	return manifest.PaychKey
}

func (s *state14) ActorVersion() actorstypes.Version {
	return actorstypes.Version14
}

func (s *state14) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
