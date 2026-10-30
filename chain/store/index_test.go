package store_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestIndexSeeks(t *testing.T) {
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	gencar, err := cg.GenesisCar()
	if err != nil {
		t.Fatal(err)
	}

	gen := cg.Genesis()

	ctx := context.TODO()

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, syncds.MutexWrap(datastore.NewMapDatastore()), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	_, _, err = cs.Import(ctx, nil, bytes.NewReader(gencar))
	if err != nil {
		t.Fatal(err)
	}

	cur := mock.TipSet(gen)

	assert.NoError(t, cs.SetGenesis(ctx, gen))

	// Put 113 blocks from genesis
	for i := 0; i < 113; i++ {
		nextBlk := mock.MkBlock(cur, 1, 1)
		nextts := mock.TipSet(nextBlk)
		assert.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{nextts}))
		assert.NoError(t, cs.AddToTipSetTracker(ctx, nextBlk))
		cur = nextts
	}

	assert.NoError(t, cs.RefreshHeaviestTipSet(ctx, cur.Height()))

	// Put 50 null epochs + 1 block
	skip := mock.MkBlock(cur, 1, 1)
	skip.Height += 50
	skipts := mock.TipSet(skip)

	assert.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{skipts}))
	assert.NoError(t, cs.AddToTipSetTracker(ctx, skip))

	if err := cs.RefreshHeaviestTipSet(ctx, skip.Height); err != nil {
		t.Fatal(err)
	}

	ts, err := cs.GetTipsetByHeight(ctx, skip.Height-10, skipts, false)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, abi.ChainEpoch(164), ts.Height())

	for i := 0; i <= 113; i++ {
		ts3, err := cs.GetTipsetByHeight(ctx, abi.ChainEpoch(i), skipts, false)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, abi.ChainEpoch(i), ts3.Height())
	}
}

func TestGetTipsetByHeightBounded(t *testing.T) {
	ctx := context.Background()
	cg, err := gen.NewGenerator()
	require.NoError(t, err)
	gencar, err := cg.GenesisCar()
	require.NoError(t, err)

	bs := blockstore.NewMemorySync()
	ds := syncds.MutexWrap(datastore.NewMapDatastore())
	cs := store.NewChainStore(bs, bs, ds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck
	_, _, err = cs.Import(ctx, nil, bytes.NewReader(gencar))
	require.NoError(t, err)
	require.NoError(t, cs.SetGenesis(ctx, cg.Genesis()))

	cur := mock.TipSet(cg.Genesis())
	for i := 0; i < 100; i++ {
		blk := mock.MkBlock(cur, 1, 1)
		cur = mock.TipSet(blk)
		require.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{cur}))
		require.NoError(t, cs.AddToTipSetTracker(ctx, blk))
	}
	require.NoError(t, cs.RefreshHeaviestTipSet(ctx, cur.Height()))

	// A fresh store over the same blockstore starts with a cold height index.
	coldStore := func(t *testing.T) *store.ChainStore {
		cs := store.NewChainStore(bs, bs, ds, filcns.Weight, nil)
		t.Cleanup(func() { _ = cs.Close() })
		require.NoError(t, cs.Load(ctx))
		return cs
	}

	t.Run("budget exceeded", func(t *testing.T) {
		restore := store.SetHeightLookupBudget(time.Nanosecond)
		defer restore()

		_, err := coldStore(t).GetTipsetByHeightBounded(ctx, 1, nil, true)
		var unavailable *api.ErrResourceUnavailable
		require.ErrorAs(t, err, &unavailable)
		require.Equal(t, api.ReasonHeightIndexCold, unavailable.Reason)
		require.Equal(t, abi.ChainEpoch(1), unavailable.Epoch)
		require.False(t, errors.Is(err, context.DeadlineExceeded), "the budget is an internal detail")
	})

	t.Run("caller cancelled", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := coldStore(t).GetTipsetByHeightBounded(cctx, 1, nil, true)
		var unavailable *api.ErrResourceUnavailable
		require.ErrorAs(t, err, &unavailable)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("warm index skips budget", func(t *testing.T) {
		restore := store.SetHeightLookupBudget(time.Nanosecond)
		defer restore()

		cs := coldStore(t)
		require.NoError(t, cs.WarmHeightIndexSync(ctx))
		cs.FailHeightIndexLoadsOnDone()
		ts, err := cs.GetTipsetByHeightBounded(ctx, 1, nil, true)
		require.NoError(t, err)
		require.Equal(t, abi.ChainEpoch(1), ts.Height())
	})

	t.Run("within budget", func(t *testing.T) {
		ts, err := coldStore(t).GetTipsetByHeightBounded(ctx, 1, nil, true)
		require.NoError(t, err)
		require.Equal(t, abi.ChainEpoch(1), ts.Height())
	})
}
