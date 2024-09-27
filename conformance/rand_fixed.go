package conformance

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/types"
)

type fixedRand struct{}

var _ rand.Rand = (*fixedRand)(nil)

// NewFixedRand creates a test vm.Rand that always returns fixed bytes value
// of utf-8 string 'i_am_random_____i_am_random_____'.
func NewFixedRand() rand.Rand {
	return &fixedRand{}
}

func (r *fixedRand) GetChainRandomness(_ context.Context, _ abi.ChainEpoch) ([32]byte, error) {
	return *(*[32]byte)([]byte("i_am_random_____i_am_random_____")), nil
}

func (r *fixedRand) GetBeaconEntry(_ context.Context, _ abi.ChainEpoch) (*types.BeaconEntry, error) {
	return &types.BeaconEntry{Round: 10, Data: []byte("i_am_random_____i_am_random_____")}, nil
}

func (r *fixedRand) GetBeaconRandomness(_ context.Context, _ abi.ChainEpoch) ([32]byte, error) {
	return *(*[32]byte)([]byte("i_am_random_____i_am_random_____")), nil // 32 bytes.
}
