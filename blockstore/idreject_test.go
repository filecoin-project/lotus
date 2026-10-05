package blockstore

import (
	"context"
	"errors"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	ipld "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

func identityCid(t *testing.T, data []byte) cid.Cid {
	t.Helper()
	h, err := mh.Sum(data, mh.IDENTITY, -1)
	require.NoError(t, err)
	return cid.NewCidV1(cid.Raw, h)
}

func blake2bBlock(t *testing.T, data []byte) blocks.Block {
	t.Helper()
	h, err := mh.Sum(data, mh.BLAKE2B_MIN+31, -1)
	require.NoError(t, err)
	blk, err := blocks.NewBlockWithCid(data, cid.NewCidV1(cid.Raw, h))
	require.NoError(t, err)
	return blk
}

// A not-found would send FallbackStore to the network for the block.
func requireIdentityRejected(t *testing.T, err error, c cid.Cid) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIdentityCid), "expected ErrIdentityCid, got %v", err)
	require.Contains(t, err.Error(), c.String())
	require.False(t, ipld.IsNotFound(err), "identity rejection must not read as not-found")
}

func TestIdentityCidRejectedOnRead(t *testing.T) {
	ctx := context.Background()
	bs := RejectIdentityCids(NewMemory())

	c := identityCid(t, []byte("attacker chosen bytes"))

	_, err := bs.Get(ctx, c)
	requireIdentityRejected(t, err, c)

	err = bs.View(ctx, c, func([]byte) error {
		t.Fatal("callback must not run for an identity CID")
		return nil
	})
	requireIdentityRejected(t, err, c)

	has, err := bs.Has(ctx, c)
	requireIdentityRejected(t, err, c)
	require.False(t, has)

	sz, err := bs.GetSize(ctx, c)
	requireIdentityRejected(t, err, c)
	require.Zero(t, sz)
}

// A pre-nv16 actor code CID.
func TestPreSkyrActorCodeCidRejected(t *testing.T) {
	ctx := context.Background()
	bs := RejectIdentityCids(NewMemory())

	c, err := cid.Decode("bafkqadtgnfwc6mjpnv2wy5djonuwo")
	require.NoError(t, err)
	require.True(t, IsIdentityCid(c))

	_, err = bs.Get(ctx, c)
	requireIdentityRejected(t, err, c)
}

// Genesis CARs carry actor code CIDs as blocks, so writes are dropped, not rejected.
func TestIdentityCidWritesDropped(t *testing.T) {
	ctx := context.Background()
	mem := NewMemory()
	bs := RejectIdentityCids(mem)

	data := []byte("inline")
	c := identityCid(t, data)
	blk, err := blocks.NewBlockWithCid(data, c)
	require.NoError(t, err)

	require.NoError(t, bs.Put(ctx, blk))
	require.NoError(t, bs.PutMany(ctx, []blocks.Block{blk}))
	require.NoError(t, bs.DeleteBlock(ctx, c))
	require.NoError(t, bs.DeleteMany(ctx, []cid.Cid{c}))
	require.Empty(t, mem)

	_, err = bs.Get(ctx, c)
	requireIdentityRejected(t, err, c)
}

func TestIdentityCidDroppedFromMixedBatch(t *testing.T) {
	ctx := context.Background()
	mem := NewMemory()
	bs := RejectIdentityCids(mem)

	good := blake2bBlock(t, []byte("ordinary"))
	idData := []byte("inline")
	bad, err := blocks.NewBlockWithCid(idData, identityCid(t, idData))
	require.NoError(t, err)

	require.NoError(t, bs.PutMany(ctx, []blocks.Block{bad, good, bad}))
	require.Len(t, mem, 1)
	has, err := bs.Has(ctx, good.Cid())
	require.NoError(t, err)
	require.True(t, has)

	require.NoError(t, bs.DeleteMany(ctx, []cid.Cid{bad.Cid(), good.Cid()}))
	require.Empty(t, mem)
}

func TestOrdinaryCidUnaffected(t *testing.T) {
	ctx := context.Background()
	bs := RejectIdentityCids(NewMemory())

	data := []byte("ordinary")
	blk := blake2bBlock(t, data)
	c := blk.Cid()

	require.NoError(t, bs.Put(ctx, blk))

	has, err := bs.Has(ctx, c)
	require.NoError(t, err)
	require.True(t, has)

	got, err := bs.Get(ctx, c)
	require.NoError(t, err)
	require.Equal(t, data, got.RawData())

	sz, err := bs.GetSize(ctx, c)
	require.NoError(t, err)
	require.Equal(t, len(data), sz)

	var viewed []byte
	require.NoError(t, bs.View(ctx, c, func(b []byte) error {
		viewed = append(viewed, b...)
		return nil
	}))
	require.Equal(t, data, viewed)

	// absent ordinary blocks are still not-found
	absent := blake2bBlock(t, []byte("never stored")).Cid()
	_, err = bs.Get(ctx, absent)
	require.True(t, ipld.IsNotFound(err))
	require.False(t, errors.Is(err, ErrIdentityCid))

	require.NoError(t, bs.DeleteBlock(ctx, c))
	has, err = bs.Has(ctx, c)
	require.NoError(t, err)
	require.False(t, has)

	other := blake2bBlock(t, []byte("another"))
	require.NoError(t, bs.PutMany(ctx, []blocks.Block{blk, other}))
	require.NoError(t, bs.DeleteMany(ctx, []cid.Cid{c, other.Cid()}))
}

func TestRejectIdentityCidsIsIdempotent(t *testing.T) {
	bs := RejectIdentityCids(NewMemory())
	require.Same(t, bs, RejectIdentityCids(bs))
}

func TestIsIdentityCid(t *testing.T) {
	require.True(t, IsIdentityCid(identityCid(t, []byte("x"))))
	require.False(t, IsIdentityCid(blake2bBlock(t, []byte("x")).Cid()))
	require.False(t, IsIdentityCid(cid.Undef))
}

func TestFromDatastoreRejectsIdentityCids(t *testing.T) {
	ctx := context.Background()
	bs := FromDatastore(dssync.MutexWrap(ds.NewMapDatastore()))

	data := []byte("inline")
	c := identityCid(t, data)
	blk, err := blocks.NewBlockWithCid(data, c)
	require.NoError(t, err)

	require.NoError(t, bs.Put(ctx, blk))
	_, err = bs.Get(ctx, c)
	requireIdentityRejected(t, err, c)
}
