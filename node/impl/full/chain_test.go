package full_test

import (
	"context"
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/node/impl/full"
)

func TestChainGetNodeDoesNotWriteBlockstore(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemory()

	m := adt.MakeEmptyMap(adt.WrapStore(ctx, cbor.NewCborStore(bs)))
	v := cbg.CborInt(42)
	require.NoError(t, m.Put(abi.IntKey(7), &v))
	root, err := m.Root()
	require.NoError(t, err)

	before := len(bs)
	a := &full.ChainAPI{ExposedBlockstore: bs}
	obj, err := a.ChainGetNode(ctx, "/ipfs/"+root.String()+"/@Hi:7")
	require.NoError(t, err)
	want, err := cbor.WrapObject(uint64(42), mh.SHA2_256, 32)
	require.NoError(t, err)
	require.Equal(t, want.Cid(), obj.Cid)
	require.Len(t, bs, before, "ChainGetNode must not write to the exposed blockstore")
}
