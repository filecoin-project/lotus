package mock

import (
	"io"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
)

type mockState struct {
	from       address.Address
	to         address.Address
	settlingAt abi.ChainEpoch
	toSend     abi.TokenAmount
	lanes      map[uint64]paych.LaneState
}

func (ms *mockState) Code() cid.Cid {
	panic("paych mock does not have CID")
}

func (ms *mockState) ActorKey() string {
	return manifest.PaychKey
}

func (ms *mockState) ActorVersion() actorstypes.Version {
	panic("paych mock is unversioned")
}

func (ms *mockState) GetState() interface{} {
	panic("implement me")
}

type mockLaneState struct {
	redeemed big.Int
	nonce    uint64
}

// NewMockPayChState constructs a state for a payment channel with the set fixed values
// that satisfies the paych.State interface.
func NewMockPayChState(from address.Address,
	to address.Address,
	settlingAt abi.ChainEpoch,
	lanes map[uint64]paych.LaneState,
) paych.State {
	return &mockState{from: from, to: to, settlingAt: settlingAt, toSend: big.NewInt(0), lanes: lanes}
}

// NewMockLaneState constructs a state for a payment channel lane with the set fixed values
// that satisfies the paych.LaneState interface. Useful for populating lanes when
// calling NewMockPayChState
func NewMockLaneState(redeemed big.Int, nonce uint64) paych.LaneState {
	return &mockLaneState{redeemed, nonce}
}

func (ms *mockState) MarshalCBOR(io.Writer) error {
	panic("not implemented")
}

// Channel owner, who has funded the actor
func (ms *mockState) From() (address.Address, error) {
	return ms.from, nil
}

// Recipient of payouts from channel
func (ms *mockState) To() (address.Address, error) {
	return ms.to, nil
}

// Height at which the channel can be `Collected`
func (ms *mockState) SettlingAt() (abi.ChainEpoch, error) {
	return ms.settlingAt, nil
}

// Amount successfully redeemed through the payment channel, paid out on `Collect()`
func (ms *mockState) ToSend() (abi.TokenAmount, error) {
	return ms.toSend, nil
}

// Get total number of lanes
func (ms *mockState) LaneCount() (uint64, error) {
	return uint64(len(ms.lanes)), nil
}

// Iterate lane states
func (ms *mockState) ForEachLaneState(cb func(idx uint64, dl paych.LaneState) error) error {
	var lastErr error
	for lane, state := range ms.lanes {
		if err := cb(lane, state); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (mls *mockLaneState) Redeemed() (big.Int, error) {
	return mls.redeemed, nil
}

func (mls *mockLaneState) Nonce() (uint64, error) {
	return mls.nonce, nil
}
