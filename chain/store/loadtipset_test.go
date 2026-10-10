package store

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

// concurrencyBlockstore records the peak number of concurrent View calls.
type concurrencyBlockstore struct {
	blockstore.Blockstore
	active, peak atomic.Int32
}

func (bs *concurrencyBlockstore) View(ctx context.Context, c cid.Cid, cb func([]byte) error) error {
	n := bs.active.Add(1)
	defer bs.active.Add(-1)
	for {
		p := bs.peak.Load()
		if n <= p || bs.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	return bs.Blockstore.View(ctx, c, cb)
}

func wideTipSet(t *testing.T, n int) *types.TipSet {
	t.Helper()
	parent := mock.TipSet(mock.MkBlock(nil, 1, 1))
	blks := make([]*types.BlockHeader, n)
	for i := range blks {
		blks[i] = mock.MkBlock(parent, 1, uint64(i+2))
	}
	return mock.TipSet(blks...)
}

func TestLoadTipSetConcurrency(t *testing.T) {
	ctx := context.Background()
	bs := &concurrencyBlockstore{Blockstore: blockstore.NewMemory()}
	cs := NewChainStore(bs, bs, datastore.NewMapDatastore(), nil, nil)
	defer cs.Close() //nolint:errcheck

	ts := wideTipSet(t, types.MaxTipSetSize)
	require.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{ts}))

	loaded, err := cs.LoadTipSet(ctx, ts.Key())
	require.NoError(t, err)
	require.Equal(t, ts.Key(), loaded.Key())
	require.LessOrEqual(t, bs.peak.Load(), int32(loadTipSetConcurrency))
	require.Greater(t, bs.peak.Load(), int32(1))
}

func TestLoadTipSetCancelled(t *testing.T) {
	bs := blockstore.NewMemory()
	cs := NewChainStore(bs, bs, datastore.NewMapDatastore(), nil, nil)
	defer cs.Close() //nolint:errcheck

	ts := wideTipSet(t, 3)
	require.NoError(t, cs.PersistTipsets(context.Background(), []*types.TipSet{ts}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cs.LoadTipSet(ctx, ts.Key())
	require.ErrorIs(t, err, context.Canceled)
}
