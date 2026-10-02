package splitstore

import (
	"context"
	"errors"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	ipld "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	bstore "github.com/filecoin-project/lotus/blockstore"
)

// The splitstore is not wrapped by RejectIdentityCids, so it rejects identity CIDs itself.

func openTestSplitStore(t *testing.T) (*SplitStore, *mockStore) {
	t.Helper()
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	hot := newMockStore()
	cold := newMockStore()
	ss, err := Open(t.TempDir(), ds, hot, cold, &Config{MarkSetType: "map", UniversalColdBlocks: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	return ss, hot
}

func requireIdentityRejected(t *testing.T, err error, c cid.Cid) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.Is(err, bstore.ErrIdentityCid), "expected ErrIdentityCid, got %v", err)
	require.Contains(t, err.Error(), c.String())
	require.False(t, ipld.IsNotFound(err), "identity rejection must not read as not-found")
}

func mkIdentityBlock(t *testing.T, data []byte) blocks.Block {
	t.Helper()
	h, err := mh.Sum(data, mh.IDENTITY, -1)
	require.NoError(t, err)
	blk, err := blocks.NewBlockWithCid(data, cid.NewCidV1(cid.Raw, h))
	require.NoError(t, err)
	return blk
}

func TestSplitStoreRejectsIdentityCid(t *testing.T) {
	ctx := context.Background()
	ss, hot := openTestSplitStore(t)

	blk := mkIdentityBlock(t, []byte("attacker chosen bytes"))
	c := blk.Cid()

	_, err := ss.Get(ctx, c)
	requireIdentityRejected(t, err, c)

	has, err := ss.Has(ctx, c)
	requireIdentityRejected(t, err, c)
	require.False(t, has)

	sz, err := ss.GetSize(ctx, c)
	requireIdentityRejected(t, err, c)
	require.Zero(t, sz)

	err = ss.View(ctx, c, func([]byte) error {
		t.Fatal("callback must not run for an identity CID")
		return nil
	})
	requireIdentityRejected(t, err, c)

	require.NoError(t, ss.Put(ctx, blk))
	require.NoError(t, ss.PutMany(ctx, []blocks.Block{blk}))
	_, err = ss.Get(ctx, c)
	requireIdentityRejected(t, err, c)

	// internal accessors used by compaction
	requireIdentityRejected(t, ss.view(c, func([]byte) error { return nil }), c)
	has, err = ss.has(c)
	requireIdentityRejected(t, err, c)
	require.False(t, has)

	require.Empty(t, hot.set, "nothing may reach the hot store")
}

func TestExposedSplitStoreRejectsIdentityCid(t *testing.T) {
	ctx := context.Background()
	ss, _ := openTestSplitStore(t)
	es := ss.Expose()

	c := mkIdentityBlock(t, []byte("attacker chosen bytes")).Cid()

	_, err := es.Get(ctx, c)
	requireIdentityRejected(t, err, c)

	has, err := es.Has(ctx, c)
	requireIdentityRejected(t, err, c)
	require.False(t, has)

	sz, err := es.GetSize(ctx, c)
	requireIdentityRejected(t, err, c)
	require.Zero(t, sz)

	err = es.View(ctx, c, func([]byte) error {
		t.Fatal("callback must not run for an identity CID")
		return nil
	})
	requireIdentityRejected(t, err, c)
}

func TestSplitStoreOrdinaryCidUnaffected(t *testing.T) {
	ctx := context.Background()
	ss, hot := openTestSplitStore(t)

	blk := blocks.NewBlock([]byte("ordinary"))
	require.NoError(t, hot.Put(ctx, blk))

	has, err := ss.Has(ctx, blk.Cid())
	require.NoError(t, err)
	require.True(t, has)

	got, err := ss.Get(ctx, blk.Cid())
	require.NoError(t, err)
	require.Equal(t, blk.RawData(), got.RawData())

	sz, err := ss.GetSize(ctx, blk.Cid())
	require.NoError(t, err)
	require.Equal(t, len(blk.RawData()), sz)

	require.NoError(t, ss.View(ctx, blk.Cid(), func(b []byte) error {
		require.Equal(t, blk.RawData(), b)
		return nil
	}))
}

// Compaction and GC walks skip identity CIDs rather than read them.
func TestIsUnitaryObjectStillCoversIdentity(t *testing.T) {
	require.True(t, isUnitaryObject(mkIdentityBlock(t, []byte("x")).Cid()))
	require.True(t, isUnitaryObject(cid.NewCidV1(cid.FilCommitmentSealed, blocks.NewBlock([]byte("y")).Cid().Hash())))
	require.False(t, isUnitaryObject(blocks.NewBlock([]byte("z")).Cid()))
}
