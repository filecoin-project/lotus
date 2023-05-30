package blockstore

import (
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

func TestMemGetCodec(t *testing.T) {
	ctx := context.Background()
	bs := NewMemory()

	cborArr := []byte{0x82, 1, 2}

	h, err := mh.Sum(cborArr, mh.SHA2_256, -1)
	require.NoError(t, err)

	rawCid := cid.NewCidV1(cid.Raw, h)
	rawBlk, err := blocks.NewBlockWithCid(cborArr, rawCid)
	require.NoError(t, err)

	err = bs.Put(ctx, rawBlk)
	require.NoError(t, err)

	cborCid := cid.NewCidV1(cid.DagCBOR, h)

	cborBlk, err := bs.Get(ctx, cborCid)
	require.NoError(t, err)

	require.Equal(t, cborCid.Prefix(), cborBlk.Cid().Prefix())
	require.EqualValues(t, cborArr, cborBlk.RawData())

	// was allocated
	require.NotEqual(t, cborBlk, rawBlk)

	gotRawBlk, err := bs.Get(ctx, rawCid)
	require.NoError(t, err)

	// not allocated
	require.Equal(t, rawBlk, gotRawBlk)
}
