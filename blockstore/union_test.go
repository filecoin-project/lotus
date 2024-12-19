package blockstore

import (
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/stretchr/testify/require"
)

var (
	b0 = blocks.NewBlock([]byte("abc"))
	b1 = blocks.NewBlock([]byte("foo"))
	b2 = blocks.NewBlock([]byte("bar"))
	b3 = blocks.NewBlock([]byte("baz"))
)

func TestUnionBlockstore_Get(t *testing.T) {
	ctx := context.Background()
	m1 := NewMemory()
	m2 := NewMemory()

	_ = m1.Put(ctx, b1)
	_ = m2.Put(ctx, b2)

	u := Union(m1, m2)

	v1, err := u.Get(ctx, b1.Cid())
	require.NoError(t, err)
	require.Equal(t, b1.RawData(), v1.RawData())

	v2, err := u.Get(ctx, b2.Cid())
	require.NoError(t, err)
	require.Equal(t, b2.RawData(), v2.RawData())
}

func TestUnionBlockstore_Put_PutMany_Delete_AllKeysChan(t *testing.T) {
	ctx := context.Background()
	m1 := NewMemory()
	m2 := NewMemory()

	u := Union(m1, m2)

	err := u.Put(ctx, b0)
	require.NoError(t, err)

	var has bool

	// write was broadcasted to all stores.
	has, _ = m1.Has(ctx, b0.Cid())
	require.True(t, has)

	has, _ = m2.Has(ctx, b0.Cid())
	require.True(t, has)

	has, _ = u.Has(ctx, b0.Cid())
	require.True(t, has)

	// put many.
	err = u.PutMany(ctx, []blocks.Block{b1, b2})
	require.NoError(t, err)

	// write was broadcasted to all stores.
	has, _ = m1.Has(ctx, b1.Cid())
	require.True(t, has)

	has, _ = m1.Has(ctx, b2.Cid())
	require.True(t, has)

	has, _ = m2.Has(ctx, b1.Cid())
	require.True(t, has)

	has, _ = m2.Has(ctx, b2.Cid())
	require.True(t, has)

	// also in the union store.
	has, _ = u.Has(ctx, b1.Cid())
	require.True(t, has)

	has, _ = u.Has(ctx, b2.Cid())
	require.True(t, has)

	// deleted from all stores.
	err = u.DeleteBlock(ctx, b1.Cid())
	require.NoError(t, err)

	has, _ = u.Has(ctx, b1.Cid())
	require.False(t, has)

	has, _ = m1.Has(ctx, b1.Cid())
	require.False(t, has)

	has, _ = m2.Has(ctx, b1.Cid())
	require.False(t, has)

	// check that AllKeysChan returns b0 and b2, twice (once per backing store)
	ch, err := u.AllKeysChan(context.Background())
	require.NoError(t, err)

	var i int
	for range ch {
		i++
	}
	require.Equal(t, 4, i)
}
