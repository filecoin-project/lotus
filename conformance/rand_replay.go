package conformance

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/chain/vm"
)

type ReplayingRand struct {
	reporter Reporter
	recorded schema.Randomness
	fallback vm.Rand
}

var _ vm.Rand = (*ReplayingRand)(nil)

// NewReplayingRand replays recorded randomness when requested, falling back to
// fixed randomness if the value cannot be found; hence this is a safe
// backwards-compatible replacement for fixedRand.
func NewReplayingRand(reporter Reporter, recorded schema.Randomness) *ReplayingRand {
	return &ReplayingRand{
		reporter: reporter,
		recorded: recorded,
		fallback: NewFixedRand(),
	}
}

func (r *ReplayingRand) match(requested schema.RandomnessRule) ([]byte, bool) {
	for _, other := range r.recorded {
		if other.On.Kind == requested.Kind &&
			other.On.Epoch == requested.Epoch &&
			other.On.DomainSeparationTag == requested.DomainSeparationTag &&
			bytes.Equal(other.On.Entropy, requested.Entropy) {
			return other.Return, true
		}
	}
	return nil, false
}

func (r *ReplayingRand) GetChainRandomness(ctx context.Context, round abi.ChainEpoch) ([32]byte, error) {
	rule := schema.RandomnessRule{
		Kind:  schema.RandomnessChain,
		Epoch: int64(round),
	}

	if ret, ok := r.match(rule); ok {
		r.reporter.Logf("returning saved chain randomness: epoch=%d, result=%x", round, ret)
		return ret, nil
	}

	r.reporter.Logf("returning fallback chain randomness: epoch=%d", round)

	return r.fallback.GetChainRandomness(ctx, round)
}

func (r *ReplayingRand) GetBeaconRandomness(ctx context.Context, round abi.ChainEpoch) ([32]byte, error) {
	rule := schema.RandomnessRule{
		Kind:  schema.RandomnessBeacon,
		Epoch: int64(round),
	}

	if ret, ok := r.match(rule); ok {
		r.reporter.Logf("returning saved beacon randomness:  epoch=%d, result=%x", round, ret)
		return ret, nil
	}

	r.reporter.Logf("returning fallback beacon randomness: epoch=%d, ", round)

	return r.fallback.GetBeaconRandomness(ctx, round)
}
