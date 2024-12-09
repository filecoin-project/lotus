package blockstore

import (
	"context"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/raulk/clock"
	"github.com/stretchr/testify/require"
)

func TestTimedCacheBlockstoreSimple(t *testing.T) {
	tc := NewTimedCacheBlockstore(10 * time.Millisecond)
	mClock := clock.NewMock()
	mClock.Set(time.Now())
	tc.clock = mClock
	tc.doneRotatingCh = make(chan struct{})

	ctx := context.Background()

	_ = tc.Start(context.Background())
	mClock.Add(1) // IDK why it is needed but it makes it work

	defer func() {
		_ = tc.Stop(context.Background())
	}()

	b1 := blocks.NewBlock([]byte("foo"))
	require.NoError(t, tc.Put(ctx, b1))

	b2 := blocks.NewBlock([]byte("bar"))
	require.NoError(t, tc.Put(ctx, b2))

	b3 := blocks.NewBlock([]byte("baz"))

	b1out, err := tc.Get(ctx, b1.Cid())
	require.NoError(t, err)
	require.Equal(t, b1.RawData(), b1out.RawData())

	has, err := tc.Has(ctx, b1.Cid())
	require.NoError(t, err)
	require.True(t, has)

	mClock.Add(10 * time.Millisecond)
	<-tc.doneRotatingCh

	// We should still have everything.
	has, err = tc.Has(ctx, b1.Cid())
	require.NoError(t, err)
	require.True(t, has)

	has, err = tc.Has(ctx, b2.Cid())
	require.NoError(t, err)
	require.True(t, has)

	// extend b2, add b3.
	require.NoError(t, tc.Put(ctx, b2))
	require.NoError(t, tc.Put(ctx, b3))

	// all keys once.
	allKeys, err := tc.AllKeysChan(context.Background())
	var ks []cid.Cid
	for k := range allKeys {
		ks = append(ks, k)
	}
	require.NoError(t, err)
	require.ElementsMatch(t, ks, []cid.Cid{b1.Cid(), b2.Cid(), b3.Cid()})

	mClock.Add(10 * time.Millisecond)
	<-tc.doneRotatingCh
	// should still have b2, and b3, but not b1

	has, err = tc.Has(ctx, b1.Cid())
	require.NoError(t, err)
	require.False(t, has)

	has, err = tc.Has(ctx, b2.Cid())
	require.NoError(t, err)
	require.True(t, has)

	has, err = tc.Has(ctx, b3.Cid())
	require.NoError(t, err)
	require.True(t, has)
}
