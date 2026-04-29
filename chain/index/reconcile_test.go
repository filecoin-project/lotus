package index

import (
	"context"
	"errors"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestReconcileGapTooLarge(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))

	// Index has data at height 100 but chain head is far ahead. Build a chain
	// where GetTipSetFromKey works for walking backwards from head.
	maxReconcile := uint64(50) // small value for testing
	indexedHeight := abi.ChainEpoch(100)
	headHeight := indexedHeight + abi.ChainEpoch(maxReconcile) + 100

	cs := newDummyChainStore()

	// Build a chain of tipsets from genesis up to headHeight so the backwards
	// walk in ReconcileWithChain can resolve parents.
	tipsets := make(map[abi.ChainEpoch]*types.TipSet)
	var parentCids []cid.Cid
	for h := abi.ChainEpoch(0); h <= headHeight; h++ {
		ts := fakeTipSet(t, rng, h, parentCids)
		tipsets[h] = ts
		cs.SetTipsetByHeightAndKey(h, ts.Key(), ts)
		parentCids = ts.Key().Cids()
	}

	head := tipsets[headHeight]
	cs.SetHeaviestTipSet(head)

	// Create the indexer with maxReconcileTipsets set to the small value so
	// the backwards walk triggers the gap limit.
	si, err := NewSqliteIndexer(":memory:", cs, 0, false, maxReconcile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = si.Close() })

	insertHead(t, si, tipsets[indexedHeight], indexedHeight)

	// ReconcileWithChain should detect the gap exceeds MaxReconciliationGap
	// and return ErrBackfillRequired.
	err = si.ReconcileWithChain(ctx, head)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrBackfillRequired), "expected ErrBackfillRequired, got: %v", err)
}

func TestReconcileSmallGapSucceeds(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))

	// Gap within the limit should succeed.
	indexedHeight := abi.ChainEpoch(100)
	headHeight := indexedHeight + 10

	cs := newDummyChainStore()

	tipsets := make(map[abi.ChainEpoch]*types.TipSet)
	var parentCids []cid.Cid
	for h := abi.ChainEpoch(0); h <= headHeight; h++ {
		ts := fakeTipSet(t, rng, h, parentCids)
		tipsets[h] = ts
		cs.SetTipsetByHeightAndKey(h, ts.Key(), ts)
		parentCids = ts.Key().Cids()
	}

	head := tipsets[headHeight]
	cs.SetHeaviestTipSet(head)

	si, err := NewSqliteIndexer(":memory:", cs, 0, false, 10000)
	require.NoError(t, err)
	t.Cleanup(func() { _ = si.Close() })

	insertHead(t, si, tipsets[indexedHeight], indexedHeight)

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	// Set up a no-op messages loader since backfill will try to index tipsets
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return nil, nil
	})

	err = si.ReconcileWithChain(ctx, head)
	require.NoError(t, err)
}

func TestBackfillRequiredDegradedMode(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))

	headHeight := abi.ChainEpoch(100)
	si, _, _ := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })
	si.Start()

	// Reads should work before setting backfill required
	_, err := si.GetMsgInfo(ctx, randomCid(t, rng))
	require.True(t, errors.Is(err, ErrNotFound), "expected ErrNotFound from empty index, got: %v", err)

	_, err = si.GetCidFromHash(ctx, ethtypes.EthHash{})
	require.True(t, errors.Is(err, ErrNotFound), "expected ErrNotFound from empty index, got: %v", err)

	// Simulate degraded mode (set by ReconcileWithChain when gap is too large)
	si.needsBackfill = true

	// Read methods should return ErrBackfillRequired
	_, err = si.GetMsgInfo(ctx, randomCid(t, rng))
	require.True(t, errors.Is(err, ErrBackfillRequired), "expected ErrBackfillRequired, got: %v", err)

	_, err = si.GetCidFromHash(ctx, ethtypes.EthHash{})
	require.True(t, errors.Is(err, ErrBackfillRequired), "expected ErrBackfillRequired, got: %v", err)

	_, err = si.GetEventsForFilter(ctx, &EventFilter{MinHeight: 1, MaxHeight: 50})
	require.True(t, errors.Is(err, ErrBackfillRequired), "expected ErrBackfillRequired, got: %v", err)

	// ChainValidateIndex should still work (this is the backfill path)
	_, err = si.ChainValidateIndex(ctx, 50, false)
	require.False(t, errors.Is(err, ErrBackfillRequired), "ChainValidateIndex should not return ErrBackfillRequired, got: %v", err)
}
