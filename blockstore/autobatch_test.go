package blockstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutobatchBlockstore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ab := NewAutobatch(ctx, NewMemory(), 2)

	require.NoError(t, ab.Put(ctx, b0))
	require.NoError(t, ab.Put(ctx, b1))
	require.NoError(t, ab.Put(ctx, b2))

	v0, err := ab.Get(ctx, b0.Cid())
	require.NoError(t, err)
	require.Equal(t, b0.RawData(), v0.RawData())

	v1, err := ab.Get(ctx, b1.Cid())
	require.NoError(t, err)
	require.Equal(t, b1.RawData(), v1.RawData())

	v2, err := ab.Get(ctx, b2.Cid())
	require.NoError(t, err)
	require.Equal(t, b2.RawData(), v2.RawData())
}
