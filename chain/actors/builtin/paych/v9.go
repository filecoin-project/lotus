package paych

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	paych9 "github.com/filecoin-project/go-state-types/builtin/v9/paych"
	adt9 "github.com/filecoin-project/go-state-types/builtin/v9/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state9)(nil)

func load9(store adt.Store, root cid.Cid) (State, error) {
	out := state9{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make9(store adt.Store) (State, error) {
	out := state9{store: store}
	out.State = paych9.State{}
	return &out, nil
}

type state9 struct {
	paych9.State
	store adt.Store
	lsAmt *adt9.Array
}

// Channel owner, who has funded the actor
func (s *state9) From() (address.Address, error) {
	return s.State.From, nil
}

// Recipient of payouts from channel
func (s *state9) To() (address.Address, error) {
	return s.State.To, nil
}

// Height at which the channel can be `Collected`
func (s *state9) SettlingAt() (abi.ChainEpoch, error) {
	return s.State.SettlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (s *state9) ToSend() (abi.TokenAmount, error) {
	return s.State.ToSend, nil
}

func (s *state9) getOrLoadLsAmt() (*adt9.Array, error) {
	if s.lsAmt != nil {
		return s.lsAmt, nil
	}

	// Get the lane state from the chain
	lsamt, err := adt9.AsArray(s.store, s.State.LaneStates, paych9.LaneStatesAmtBitwidth)
	if err != nil {
		return nil, err
	}

	s.lsAmt = lsamt
	return lsamt, nil
}

// Get total number of lanes
func (s *state9) LaneCount() (uint64, error) {
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return 0, err
	}
	return lsamt.Length(), nil
}

func (s *state9) GetState() interface{} {
	return &s.State
}

// Iterate lane states
func (s *state9) ForEachLaneState(cb func(idx uint64, dl LaneState) error) error {
	// Get the lane state from the chain
	lsamt, err := s.getOrLoadLsAmt()
	if err != nil {
		return err
	}

	// Note: we use a map instead of an array to store laneStates because the
	// client sets the lane ID (the index) and potentially they could use a
	// very large index.
	var ls paych9.LaneState
	return lsamt.ForEach(&ls, func(i int64) error {
		return cb(uint64(i), &laneState9{ls})
	})
}

type laneState9 struct {
	paych9.LaneState
}

func (ls *laneState9) Redeemed() (big.Int, error) {
	return ls.LaneState.Redeemed, nil
}

func (ls *laneState9) Nonce() (uint64, error) {
	return ls.LaneState.Nonce, nil
}

func (s *state9) ActorKey() string {
	return manifest.PaychKey
}

func (s *state9) ActorVersion() actorstypes.Version {
	return actorstypes.Version9
}

func (s *state9) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
