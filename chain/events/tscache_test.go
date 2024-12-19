package events

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

type tsCacheAPIFailOnStorageCall struct {
	t *testing.T
}

func (tc *tsCacheAPIFailOnStorageCall) ChainGetTipSetAfterHeight(ctx context.Context, epoch abi.ChainEpoch, key types.TipSetKey) (*types.TipSet, error) {
	tc.t.Fatal("storage call")
	return &types.TipSet{}, nil
}

func (tc *tsCacheAPIFailOnStorageCall) ChainGetTipSetByHeight(ctx context.Context, epoch abi.ChainEpoch, key types.TipSetKey) (*types.TipSet, error) {
	tc.t.Fatal("storage call")
	return &types.TipSet{}, nil
}
func (tc *tsCacheAPIFailOnStorageCall) ChainHead(ctx context.Context) (*types.TipSet, error) {
	tc.t.Fatal("storage call")
	return &types.TipSet{}, nil
}
func (tc *tsCacheAPIFailOnStorageCall) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	tc.t.Fatal("storage call")
	return &types.TipSet{}, nil
}

type cacheHarness struct {
	t *testing.T

	miner  address.Address
	tsc    *tipSetCache
	height abi.ChainEpoch
}

func newCacheharness(t *testing.T) *cacheHarness {
	a, err := address.NewFromString("t00")
	require.NoError(t, err)

	h := &cacheHarness{
		t:      t,
		tsc:    newTSCache(&tsCacheAPIFailOnStorageCall{t: t}, 50),
		height: 75,
		miner:  a,
	}
	h.addWithParents(nil)
	return h
}

func (h *cacheHarness) addWithParents(parents []cid.Cid) {
	ts, err := types.NewTipSet([]*types.BlockHeader{{
		Miner:                 h.miner,
		Height:                h.height,
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		Parents:               parents,
	}})
	require.NoError(h.t, err)
	require.NoError(h.t, h.tsc.add(ts))
	h.height++
}

func (h *cacheHarness) add() {
	last, err := h.tsc.ChainHead(context.Background())
	require.NoError(h.t, err)
	h.addWithParents(last.Cids())
}

func (h *cacheHarness) revert() {
	best, err := h.tsc.ChainHead(context.Background())
	require.NoError(h.t, err)
	err = h.tsc.revert(best)
	require.NoError(h.t, err)
	h.height--
}

func (h *cacheHarness) skip(n abi.ChainEpoch) {
	h.height += n
}

func TestTsCache(t *testing.T) {
	h := newCacheharness(t)

	for i := 0; i < 9000; i++ {
		if i%90 > 60 {
			h.revert()
		} else {
			h.add()
		}
	}
}

func TestTsCacheNulls(t *testing.T) {
	ctx := context.Background()
	h := newCacheharness(t)

	h.add()
	h.add()
	h.add()
	h.skip(5)

	h.add()
	h.add()

	best, err := h.tsc.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, h.height-1, best.Height())

	ts, err := h.tsc.ChainGetTipSetByHeight(ctx, h.height-1, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, h.height-1, ts.Height())

	ts, err = h.tsc.ChainGetTipSetByHeight(ctx, h.height-2, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, h.height-2, ts.Height())

	// Should skip the nulls and walk back to the last tipset.
	ts, err = h.tsc.ChainGetTipSetByHeight(ctx, h.height-3, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, h.height-8, ts.Height())

	ts, err = h.tsc.ChainGetTipSetByHeight(ctx, h.height-8, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, h.height-8, ts.Height())

	best, err = h.tsc.ChainHead(ctx)
	require.NoError(t, err)
	require.NoError(t, h.tsc.revert(best))

	best, err = h.tsc.ChainHead(ctx)
	require.NoError(t, err)
	require.NoError(t, h.tsc.revert(best))

	best, err = h.tsc.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, h.height-8, best.Height())

	h.skip(50)
	h.add()

	ts, err = h.tsc.ChainGetTipSetByHeight(ctx, h.height-1, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, h.height-1, ts.Height())
}

type tsCacheAPIStorageCallCounter struct {
	t                         *testing.T
	chainGetTipSetByHeight    int
	chainGetTipSetAfterHeight int
	chainGetTipSet            int
	chainHead                 int
}

func (tc *tsCacheAPIStorageCallCounter) ChainGetTipSetByHeight(ctx context.Context, epoch abi.ChainEpoch, key types.TipSetKey) (*types.TipSet, error) {
	tc.chainGetTipSetByHeight++
	return &types.TipSet{}, nil
}
func (tc *tsCacheAPIStorageCallCounter) ChainGetTipSetAfterHeight(ctx context.Context, epoch abi.ChainEpoch, key types.TipSetKey) (*types.TipSet, error) {
	tc.chainGetTipSetAfterHeight++
	return &types.TipSet{}, nil
}
func (tc *tsCacheAPIStorageCallCounter) ChainHead(ctx context.Context) (*types.TipSet, error) {
	tc.chainHead++
	return &types.TipSet{}, nil
}
func (tc *tsCacheAPIStorageCallCounter) ChainGetTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	tc.chainGetTipSet++
	return &types.TipSet{}, nil
}

func TestTsCacheEmpty(t *testing.T) {
	// Calling best on an empty cache should just call out to the chain API
	callCounter := &tsCacheAPIStorageCallCounter{t: t}
	tsc := newTSCache(callCounter, 50)
	_, err := tsc.ChainHead(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, callCounter.chainHead)
}

func TestTsCacheSkip(t *testing.T) {
	h := newCacheharness(t)

	ts, err := types.NewTipSet([]*types.BlockHeader{{
		Miner:                 h.miner,
		Height:                h.height,
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		// With parents that don't match the last block.
		Parents: nil,
	}})
	require.NoError(h.t, err)
	err = h.tsc.add(ts)
	require.Error(t, err)
}
