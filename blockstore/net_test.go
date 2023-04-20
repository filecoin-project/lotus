package blockstore

import (
	"context"
	"fmt"
	"io"
	"testing"

	block "github.com/ipfs/go-block-format"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-msgio"
	"github.com/stretchr/testify/require"
)

func TestNetBstore(t *testing.T) {
	ctx := context.Background()

	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	cm := msgio.Combine(msgio.NewWriter(cw), msgio.NewReader(cr))
	sm := msgio.Combine(msgio.NewWriter(sw), msgio.NewReader(sr))

	bbs := NewMemorySync()
	_ = HandleNetBstoreStream(ctx, bbs, sm)

	nbs := NewNetworkStore(cm)

	tb1 := block.NewBlock([]byte("aoeu"))

	h, err := nbs.Has(ctx, tb1.Cid())
	require.NoError(t, err)
	require.False(t, h)

	err = nbs.Put(ctx, tb1)
	require.NoError(t, err)

	h, err = nbs.Has(ctx, tb1.Cid())
	require.NoError(t, err)
	require.True(t, h)

	sz, err := nbs.GetSize(ctx, tb1.Cid())
	require.NoError(t, err)
	require.Equal(t, 4, sz)

	err = nbs.DeleteBlock(ctx, tb1.Cid())
	require.NoError(t, err)

	h, err = nbs.Has(ctx, tb1.Cid())
	require.NoError(t, err)
	require.False(t, h)

	_, err = nbs.Get(ctx, tb1.Cid())
	fmt.Println(err)
	require.True(t, ipld.IsNotFound(err))

	err = nbs.Put(ctx, tb1)
	require.NoError(t, err)

	b, err := nbs.Get(ctx, tb1.Cid())
	require.NoError(t, err)
	require.Equal(t, "aoeu", string(b.RawData()))
}
