package rand

import (
	"context"
	"math/rand"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
)

// fakeRand implements the vm.Rand interface and returns fake randomness
// for each epoch instead of the real randomness provided by Drand.
// Use fake randomness with care as, while it allows you to decouple storage proofs
// from other processes in the network, it also lowers the security of all
// the proof-related protocols.
// We are currently using this fake randomness to decouple storage processes from
// consensus.
// TODO: Integrate real randomness into alternative consensus implementations in order to
// support "real" verifiable storage?
type fakeRand struct{}

func (fr *fakeRand) GetChainRandomness(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, err := rand.New(rand.NewSource(int64(randEpoch * 1000))).Read(out) //nolint
	return out, err
}

func (fr *fakeRand) GetBeaconRandomness(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte) ([]byte, error) {
	out := make([]byte, 32)
	_, err := rand.New(rand.NewSource(int64(randEpoch))).Read(out) //nolint
	return out, err
}
