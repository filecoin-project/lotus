package conformance

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/vm"
)

type fixedRand struct{}

var _ vm.Rand = (*fixedRand)(nil)

// NewFixedRand creates a test vm.Rand that always returns fixed bytes value
// of utf-8 string 'i_am_random_____i_am_random_____'.
func NewFixedRand() vm.Rand {
	return &fixedRand{}
}

func (r *fixedRand) GetChainRandomnessLookingForward(_ context.Context, _ crypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}

func (r *fixedRand) GetChainRandomnessLookingBack(_ context.Context, _ crypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}

func (r *fixedRand) GetBeaconRandomnessLookingForward(_ context.Context, _ crypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}

func (r *fixedRand) GetBeaconRandomnessLookingBack(_ context.Context, _ crypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}

func (r *fixedRand) GetLatestBeaconRandomnessLookingForward(_ context.Context, _ crypto.DomainSeparationTag, _ abi.ChainEpoch, _ []byte) ([]byte, error) {
	return []byte("i_am_random_____i_am_random_____"), nil // 32 bytes.
}
