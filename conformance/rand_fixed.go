package conformance

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/vm"
)

type fixedRand struct{}

var _ vm.Rand = (*fixedRand)(nil)

// NewFixedRand creates a test vm.Rand that always returns fixed bytes value
// of utf-8 string 'i_am_random_____i_am_random_____'.
func NewFixedRand() vm.Rand {
	return &fixedRand{}
}

func (r *fixedRand) GetChainRandomness(_ context.Context, _ abi.ChainEpoch) ([32]byte, error) {
	return *(*[32]byte)([]byte("i_am_random_____i_am_random_____")), nil
}

func (r *fixedRand) GetBeaconRandomness(_ context.Context, _ abi.ChainEpoch) ([32]byte, error) {
	return *(*[32]byte)([]byte("i_am_random_____i_am_random_____")), nil // 32 bytes.
}
