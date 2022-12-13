package shared_testutil

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blocksutil "github.com/ipfs/go-ipfs-blocksutil"
	"github.com/jbenet/go-random"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	"github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

var blockGenerator = blocksutil.NewBlockGenerator()

//var prioritySeq int
var seedSeq int64

// RandomBytes returns a byte array of the given size with random values.
func RandomBytes(n int64) []byte {
	data := new(bytes.Buffer)
	random.WritePseudoRandomBytes(n, data, seedSeq) // nolint: gosec,errcheck
	seedSeq++
	return data.Bytes()
}

// GenerateBlocksOfSize generates a series of blocks of the given byte size
func GenerateBlocksOfSize(n int, size int64) []blocks.Block {
	generatedBlocks := make([]blocks.Block, 0, n)
	for i := 0; i < n; i++ {
		b := blocks.NewBlock(RandomBytes(size))
		generatedBlocks = append(generatedBlocks, b)

	}
	return generatedBlocks
}

// GenerateCids produces n content identifiers.
func GenerateCids(n int) []cid.Cid {
	cids := make([]cid.Cid, 0, n)
	for i := 0; i < n; i++ {
		c := blockGenerator.Next().Cid()
		cids = append(cids, c)
	}
	return cids
}

var peerSeq int

// GeneratePeers creates n peer ids.
func GeneratePeers(n int) []peer.ID {
	peerIds := make([]peer.ID, 0, n)
	for i := 0; i < n; i++ {
		peerSeq++
		p := peer.ID(fmt.Sprint(peerSeq))
		peerIds = append(peerIds, p)
	}
	return peerIds
}

// ContainsPeer returns true if a peer is found n a list of peers.
func ContainsPeer(peers []peer.ID, p peer.ID) bool {
	for _, n := range peers {
		if p == n {
			return true
		}
	}
	return false
}

// IndexOf returns the index of a given cid in an array of blocks
func IndexOf(blks []blocks.Block, c cid.Cid) int {
	for i, n := range blks {
		if n.Cid() == c {
			return i
		}
	}
	return -1
}

// ContainsBlock returns true if a block is found n a list of blocks
func ContainsBlock(blks []blocks.Block, block blocks.Block) bool {
	return IndexOf(blks, block.Cid()) != -1
}

// TestVoucherEquality verifies that two vouchers are equal to one another
func TestVoucherEquality(t *testing.T, a, b *paych.SignedVoucher) {
	aB, err := cborutil.Dump(a)
	require.NoError(t, err)
	bB, err := cborutil.Dump(b)
	require.NoError(t, err)
	require.True(t, bytes.Equal(aB, bB))
}

// AssertDealState asserts equality of StorageDealStatus but with better error messaging
func AssertDealState(t *testing.T, expected storagemarket.StorageDealStatus, actual storagemarket.StorageDealStatus) {
	assert.Equal(t, expected, actual,
		"Unexpected deal status\nexpected: %s (%d)\nactual  : %s (%d)",
		storagemarket.DealStates[expected], expected,
		storagemarket.DealStates[actual], actual,
	)
}

func AssertRetrievalDealState(t *testing.T, expected, actual retrievalmarket.DealStatus) {
	assert.Equal(t, expected, actual,
		"Unexpected retrieval deal status:\nexpected: %s (%d)\nactual  : %s (%d)",
		retrievalmarket.DealStatuses[expected], expected,
		retrievalmarket.DealStatuses[actual], actual,
	)
}

func GenerateCid(t *testing.T, o interface{}) cid.Cid {
	node, err := cborutil.AsIpld(o)
	assert.NoError(t, err)
	return node.Cid()
}

func DatastoreAtVersion(t *testing.T, ds datastore.Batching, version versioning.VersionKey) datastore.Batching {
	err := ds.Put(context.TODO(), datastore.NewKey("/versions/current"), []byte(version))
	require.NoError(t, err)
	return namespace.Wrap(ds, datastore.NewKey(fmt.Sprintf("/%s", version)))
}

func NewIDAddr(t testing.TB, id uint64) address.Address {
	ret, err := address.NewIDAddress(id)
	require.NoError(t, err)
	return ret
}
