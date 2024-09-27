package conformance

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/types"
)

type RecordingRand struct {
	reporter Reporter
	api      v1api.FullNode

	// once guards the loading of the head tipset.
	// can be removed when https://github.com/filecoin-project/lotus/issues/4223
	// is fixed.
	once     sync.Once
	head     types.TipSetKey
	lk       sync.Mutex
	recorded schema.Randomness
}

var _ rand.Rand = (*RecordingRand)(nil)

// NewRecordingRand returns a vm.Rand implementation that proxies calls to a
// full Lotus node via JSON-RPC, and records matching rules and responses so
// they can later be embedded in test vectors.
func NewRecordingRand(reporter Reporter, api v1api.FullNode) *RecordingRand {
	return &RecordingRand{reporter: reporter, api: api}
}

func (r *RecordingRand) loadHead() {
	head, err := r.api.ChainHead(context.Background())
	if err != nil {
		panic(fmt.Sprintf("could not fetch chain head while fetching randomness: %s", err))
	}
	r.head = head.Key()
}

func (r *RecordingRand) GetChainRandomness(ctx context.Context, round abi.ChainEpoch) ([32]byte, error) {
	r.once.Do(r.loadHead)
	// FullNode's v1 ChainGetRandomnessFromTickets handles whether we should be looking forward or back
	ret, err := r.api.StateGetRandomnessDigestFromTickets(ctx, round, r.head)
	if err != nil {
		return [32]byte{}, err
	}

	r.reporter.Logf("fetched and recorded chain randomness for: epoch=%d, result=%x", round, ret)

	match := schema.RandomnessMatch{
		On: schema.RandomnessRule{
			Kind:  schema.RandomnessChain,
			Epoch: int64(round),
		},
		Return: []byte(ret),
	}
	r.lk.Lock()
	r.recorded = append(r.recorded, match)
	r.lk.Unlock()

	return *(*[32]byte)(ret), err
}

func (r *RecordingRand) GetBeaconRandomness(ctx context.Context, round abi.ChainEpoch) ([32]byte, error) {
	r.once.Do(r.loadHead)
	ret, err := r.api.StateGetRandomnessDigestFromBeacon(ctx, round, r.head)
	if err != nil {
		return [32]byte{}, err
	}

	r.reporter.Logf("fetched and recorded beacon randomness for: epoch=%d, result=%x", round, ret)

	match := schema.RandomnessMatch{
		On: schema.RandomnessRule{
			Kind:  schema.RandomnessBeacon,
			Epoch: int64(round),
		},
		Return: []byte(ret),
	}
	r.lk.Lock()
	r.recorded = append(r.recorded, match)
	r.lk.Unlock()

	return *(*[32]byte)(ret), err
}

func (r *RecordingRand) GetBeaconEntry(ctx context.Context, round abi.ChainEpoch) (*types.BeaconEntry, error) {
	r.once.Do(r.loadHead)
	ret, err := r.api.StateGetBeaconEntry(ctx, round)
	if err != nil {
		return nil, err
	}

	r.reporter.Logf("fetched and recorded beacon randomness for: epoch=%d, result=%x", round, ret)

	match := schema.RandomnessMatch{
		On: schema.RandomnessRule{
			Kind:  schema.RandomnessBeacon,
			Epoch: int64(round),
		},
		Return: ret.Data,
	}
	r.lk.Lock()
	r.recorded = append(r.recorded, match)
	r.lk.Unlock()

	return ret, err
}

func (r *RecordingRand) Recorded() schema.Randomness {
	r.lk.Lock()
	defer r.lk.Unlock()

	return r.recorded
}
