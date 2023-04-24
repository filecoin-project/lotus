package blockrecorder_test

import (
	"bytes"
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipld/go-car"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/blockrecorder"
)

func TestBlockRecording(t *testing.T) {
	testData := shared_testutil.NewTestIPLDTree()
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	node := ssb.ExploreFields(func(efsb builder.ExploreFieldsSpecBuilder) {
		efsb.Insert("linkedMap",
			ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreRecursiveEdge())))
	}).Node()

	ctx := context.Background()
	sc := car.NewSelectiveCar(ctx, testData, []car.Dag{
		car.Dag{
			Root:     testData.RootNodeLnk.(cidlink.Link).Cid,
			Selector: node,
		},
	})

	carBuf := new(bytes.Buffer)
	blockLocationBuf := new(bytes.Buffer)
	err := sc.Write(carBuf, blockrecorder.RecordEachBlockTo(blockLocationBuf))
	require.NoError(t, err)

	metadata, err := blockrecorder.ReadBlockMetadata(blockLocationBuf)
	require.NoError(t, err)

	blks := []blocks.Block{
		testData.LeafAlphaBlock,
		testData.MiddleMapBlock,
		testData.RootBlock,
	}
	carBytes := carBuf.Bytes()
	for _, blk := range blks {
		cid := blk.Cid()
		var found bool
		var metadatum blockrecorder.PieceBlockMetadata
		for _, testMetadatum := range metadata {
			if testMetadatum.CID.Equals(cid) {
				metadatum = testMetadatum
				found = true
				break
			}
		}
		require.True(t, found)
		testBuf := carBytes[metadatum.Offset : metadatum.Offset+metadatum.Size]
		require.Equal(t, blk.RawData(), testBuf)
	}
	missingBlks := []blocks.Block{
		testData.LeafBetaBlock,
		testData.MiddleListBlock,
	}
	for _, blk := range missingBlks {
		cid := blk.Cid()
		var found bool
		for _, testMetadatum := range metadata {
			if testMetadatum.CID.Equals(cid) {
				found = true
				break
			}
		}
		require.False(t, found)
	}
}
