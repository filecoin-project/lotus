package store_test

import (
	"bytes"
	"context"
	"math"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/exitcode"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

const v2CountField = 1 // [height, count, node]

func putRaw(t *testing.T, bs blockstore.Blockstore, data []byte) cid.Cid {
	c, err := cid.Prefix{Version: 1, Codec: cid.DagCBOR, MhType: mh.BLAKE2B_MIN + 31, MhLength: -1}.Sum(data)
	require.NoError(t, err)
	blk, err := blocks.NewBlockWithCid(data, c)
	require.NoError(t, err)
	require.NoError(t, bs.Put(context.Background(), blk))
	return c
}

// setRootCount re-encodes an AMT root block with a replacement count and
// stores it under a new CID.
func setRootCount(t *testing.T, bs blockstore.Blockstore, root cid.Cid, field int, count uint64) cid.Cid {
	blk, err := bs.Get(context.Background(), root)
	require.NoError(t, err)

	cr := cbg.NewCborReader(bytes.NewReader(blk.RawData()))
	maj, n, err := cr.ReadHeader()
	require.NoError(t, err)
	require.Equal(t, byte(cbg.MajArray), maj)

	var buf bytes.Buffer
	cw := cbg.NewCborWriter(&buf)
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajArray, n))
	for i := 0; i < int(n); i++ {
		var d cbg.Deferred
		require.NoError(t, d.UnmarshalCBOR(cr))
		if i == field {
			require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, count))
			continue
		}
		_, err := buf.Write(d.Raw)
		require.NoError(t, err)
	}
	return putRaw(t, bs, buf.Bytes())
}

// v2Node encodes a legacy AMT node [bmap, links, values] with no values.
func v2Node(t *testing.T, bmap byte, links []cid.Cid) []byte {
	var buf bytes.Buffer
	cw := cbg.NewCborWriter(&buf)
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajArray, 3))
	require.NoError(t, cbg.WriteByteArray(cw, []byte{bmap}))
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajArray, uint64(len(links))))
	for _, l := range links {
		require.NoError(t, cbg.WriteCid(cw, l))
	}
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajArray, 0))
	return buf.Bytes()
}

// emptySubtreeV2Root builds a height-20 legacy AMT whose every link slot shares
// the next level's single node, ending in an empty leaf: 8^20 paths and no
// values from 21 blocks.
func emptySubtreeV2Root(t *testing.T, bs blockstore.Blockstore, count uint64) cid.Cid {
	const height = 20
	child := putRaw(t, bs, v2Node(t, 0, nil))
	links := make([]cid.Cid, 8)
	for h := 1; h < height; h++ {
		for i := range links {
			links[i] = child
		}
		child = putRaw(t, bs, v2Node(t, 0xff, links))
	}
	for i := range links {
		links[i] = child
	}

	var buf bytes.Buffer
	cw := cbg.NewCborWriter(&buf)
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajArray, 3))
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, height))
	require.NoError(t, cw.WriteMajorTypeHeader(cbg.MajUnsignedInt, count))
	_, err := buf.Write(v2Node(t, 0xff, links))
	require.NoError(t, err)
	return putRaw(t, bs, buf.Bytes())
}

func TestReadReceipts(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemory()
	cs := store.NewChainStore(bs, bs, datastore.NewMapDatastore(), filcns.Weight, nil)

	receiptsRoot := func(idx ...uint64) cid.Cid {
		arr := blockadt.MakeEmptyArray(cs.ActorStore(ctx))
		for _, i := range idx {
			require.NoError(t, arr.Set(i, &types.MessageReceipt{ExitCode: exitcode.Ok, GasUsed: 1}))
		}
		root, err := arr.Root()
		require.NoError(t, err)
		return root
	}

	t.Run("round trip", func(t *testing.T) {
		receipts := []types.MessageReceipt{
			{ExitCode: exitcode.Ok, Return: []byte{1}, GasUsed: 100},
			{ExitCode: exitcode.ErrForbidden, GasUsed: 200},
			{ExitCode: exitcode.Ok, GasUsed: 300},
		}
		arr := blockadt.MakeEmptyArray(cs.ActorStore(ctx))
		for i := range receipts {
			require.NoError(t, arr.Set(uint64(i), &receipts[i]))
		}
		root, err := arr.Root()
		require.NoError(t, err)

		got, err := cs.ReadReceipts(ctx, root, 0)
		require.NoError(t, err)
		require.Equal(t, receipts, got)

		got, err = cs.ReadReceipts(ctx, root, 1<<10)
		require.NoError(t, err)
		require.Equal(t, receipts, got)

		_, err = cs.ReadReceipts(ctx, root, 64)
		require.ErrorContains(t, err, "receipts exceed")
	})

	t.Run("empty", func(t *testing.T) {
		got, err := cs.ReadReceipts(ctx, receiptsRoot(), 0)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})

	t.Run("count above entries", func(t *testing.T) {
		root := setRootCount(t, bs, receiptsRoot(0, 1), v2CountField, 3)
		_, err := cs.ReadReceipts(ctx, root, 0)
		require.ErrorContains(t, err, "amt entry 2 of 3 not found")

		root = setRootCount(t, bs, receiptsRoot(0), v2CountField, math.MaxUint64)
		_, err = cs.ReadReceipts(ctx, root, 1<<20)
		require.ErrorContains(t, err, "amt entry 1 of")
	})

	t.Run("sparse", func(t *testing.T) {
		root := setRootCount(t, bs, receiptsRoot(5), v2CountField, 1)
		_, err := cs.ReadReceipts(ctx, root, 0)
		require.ErrorContains(t, err, "amt entry 0 of 1 not found")
	})

	t.Run("shared leaf nodes", func(t *testing.T) {
		idx := make([]uint64, 64)
		for i := range idx {
			idx[i] = uint64(i)
		}
		root := setRootCount(t, bs, receiptsRoot(idx...), v2CountField, math.MaxUint64)
		_, err := cs.ReadReceipts(ctx, root, 1<<10)
		require.ErrorContains(t, err, "receipts exceed")
	})

	t.Run("shared empty subtrees", func(t *testing.T) {
		got, err := cs.ReadReceipts(ctx, emptySubtreeV2Root(t, bs, 0), 0)
		require.NoError(t, err)
		require.Empty(t, got)

		_, err = cs.ReadReceipts(ctx, emptySubtreeV2Root(t, bs, 1), 0)
		require.ErrorContains(t, err, "amt entry 0 of 1 not found")
	})
}

func TestReadMsgMetaCids(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemory()
	cs := store.NewChainStore(bs, bs, datastore.NewMapDatastore(), filcns.Weight, nil)
	cst := cbor.NewCborStore(bs)

	msgCid, err := cid.Prefix{Version: 1, Codec: cid.DagCBOR, MhType: mh.SHA2_256, MhLength: -1}.Sum([]byte("msg"))
	require.NoError(t, err)
	cidsRoot := func(n int) cid.Cid {
		arr := blockadt.MakeEmptyArray(cs.ActorStore(ctx))
		c := cbg.CborCid(msgCid)
		for i := 0; i < n; i++ {
			require.NoError(t, arr.Set(uint64(i), &c))
		}
		root, err := arr.Root()
		require.NoError(t, err)
		return root
	}
	readMeta := func(bls, secpk cid.Cid) ([]cid.Cid, []cid.Cid, error) {
		mm, err := cst.Put(ctx, &types.MsgMeta{BlsMessages: bls, SecpkMessages: secpk})
		require.NoError(t, err)
		return cs.ReadMsgMetaCids(ctx, mm)
	}
	empty := cidsRoot(0)

	t.Run("round trip", func(t *testing.T) {
		bls, secpk, err := readMeta(cidsRoot(3), cidsRoot(2))
		require.NoError(t, err)
		require.Len(t, bls, 3)
		require.Len(t, secpk, 2)
	})

	t.Run("count above block message limit", func(t *testing.T) {
		root := setRootCount(t, bs, cidsRoot(1), v2CountField, uint64(buildconstants.BlockMessageLimit)+1)
		_, _, err := readMeta(root, empty)
		require.ErrorContains(t, err, "limit is")
	})

	t.Run("combined messages above block message limit", func(t *testing.T) {
		half := cidsRoot(buildconstants.BlockMessageLimit/2 + 1)
		_, _, err := readMeta(half, half)
		require.ErrorContains(t, err, "block has")
	})

	t.Run("count above entries", func(t *testing.T) {
		_, _, err := readMeta(setRootCount(t, bs, cidsRoot(1), v2CountField, 2), empty)
		require.ErrorContains(t, err, "amt entry 1 of 2 not found")
	})

	t.Run("shared empty subtrees", func(t *testing.T) {
		bls, _, err := readMeta(emptySubtreeV2Root(t, bs, 0), empty)
		require.NoError(t, err)
		require.Empty(t, bls)
	})
}
