package shared_testutil

import (
	"bytes"
	"context"
	"errors"
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-car"
	"github.com/ipld/go-ipld-prime"

	// to register multicodec
	_ "github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/fluent"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
)

// TestIPLDTree is a set of IPLD Data that forms a tree spread across some blocks
// with a serialized in memory representation
type TestIPLDTree struct {
	Storage           map[ipld.Link][]byte
	LeafAlpha         ipld.Node
	LeafAlphaLnk      ipld.Link
	LeafAlphaBlock    blocks.Block
	LeafBeta          ipld.Node
	LeafBetaLnk       ipld.Link
	LeafBetaBlock     blocks.Block
	MiddleMapNode     ipld.Node
	MiddleMapNodeLnk  ipld.Link
	MiddleMapBlock    blocks.Block
	MiddleListNode    ipld.Node
	MiddleListNodeLnk ipld.Link
	MiddleListBlock   blocks.Block
	RootNode          ipld.Node
	RootNodeLnk       ipld.Link
	RootBlock         blocks.Block
}

// NewTestIPLDTree returns a fake tree of nodes, spread across 5 blocks
func NewTestIPLDTree() TestIPLDTree {
	var storage = make(map[ipld.Link][]byte)
	encode := func(n ipld.Node) (ipld.Node, ipld.Link) {
		lb := cidlink.LinkPrototype{Prefix: cid.Prefix{
			Version:  1,
			Codec:    0x0129,
			MhType:   0x13,
			MhLength: 4,
		}}
		lsys := cidlink.DefaultLinkSystem()
		lsys.StorageWriteOpener = func(ipld.LinkContext) (io.Writer, ipld.BlockWriteCommitter, error) {
			buf := bytes.Buffer{}
			return &buf, func(lnk ipld.Link) error {
				storage[lnk] = buf.Bytes()
				return nil
			}, nil
		}
		lnk, err := lsys.Store(ipld.LinkContext{}, lb, n)
		if err != nil {
			panic(err)
		}
		return n, lnk
	}

	var (
		leafAlpha, leafAlphaLnk         = encode(fluent.MustBuild(basicnode.Prototype.String, func(na fluent.NodeAssembler) { na.AssignString("alpha") }))
		leafAlphaBlock, _               = blocks.NewBlockWithCid(storage[leafAlphaLnk], leafAlphaLnk.(cidlink.Link).Cid)
		leafBeta, leafBetaLnk           = encode(fluent.MustBuild(basicnode.Prototype.String, func(na fluent.NodeAssembler) { na.AssignString("beta") }))
		leafBetaBlock, _                = blocks.NewBlockWithCid(storage[leafBetaLnk], leafBetaLnk.(cidlink.Link).Cid)
		middleMapNode, middleMapNodeLnk = encode(fluent.MustBuildMap(basicnode.Prototype.Map, 3, func(ma fluent.MapAssembler) {
			ma.AssembleEntry("foo").AssignBool(true)
			ma.AssembleEntry("bar").AssignBool(false)
			ma.AssembleEntry("nested").CreateMap(2, func(ma fluent.MapAssembler) {
				ma.AssembleEntry("alink").AssignLink(leafAlphaLnk)
				ma.AssembleEntry("nonlink").AssignString("zoo")
			})
		}))
		middleMapBlock, _                 = blocks.NewBlockWithCid(storage[middleMapNodeLnk], middleMapNodeLnk.(cidlink.Link).Cid)
		middleListNode, middleListNodeLnk = encode(fluent.MustBuildList(basicnode.Prototype.List, 4, func(la fluent.ListAssembler) {
			la.AssembleValue().AssignLink(leafAlphaLnk)
			la.AssembleValue().AssignLink(leafAlphaLnk)
			la.AssembleValue().AssignLink(leafBetaLnk)
			la.AssembleValue().AssignLink(leafAlphaLnk)
		}))
		middleListBlock, _    = blocks.NewBlockWithCid(storage[middleListNodeLnk], middleListNodeLnk.(cidlink.Link).Cid)
		rootNode, rootNodeLnk = encode(fluent.MustBuildMap(basicnode.Prototype.Map, 4, func(ma fluent.MapAssembler) {
			ma.AssembleEntry("plain").AssignString("olde string")
			ma.AssembleEntry("linkedString").AssignLink(leafAlphaLnk)
			ma.AssembleEntry("linkedMap").AssignLink(middleMapNodeLnk)
			ma.AssembleEntry("linkedList").AssignLink(middleListNodeLnk)
		}))
		rootBlock, _ = blocks.NewBlockWithCid(storage[rootNodeLnk], rootNodeLnk.(cidlink.Link).Cid)
	)
	return TestIPLDTree{
		Storage:           storage,
		LeafAlpha:         leafAlpha,
		LeafAlphaLnk:      leafAlphaLnk,
		LeafAlphaBlock:    leafAlphaBlock,
		LeafBeta:          leafBeta,
		LeafBetaLnk:       leafBetaLnk,
		LeafBetaBlock:     leafBetaBlock,
		MiddleMapNode:     middleMapNode,
		MiddleMapNodeLnk:  middleMapNodeLnk,
		MiddleMapBlock:    middleMapBlock,
		MiddleListNode:    middleListNode,
		MiddleListNodeLnk: middleListNodeLnk,
		MiddleListBlock:   middleListBlock,
		RootNode:          rootNode,
		RootNodeLnk:       rootNodeLnk,
		RootBlock:         rootBlock,
	}
}

// Get makes a test tree behave like a block read store
func (tt TestIPLDTree) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	data, ok := tt.Storage[cidlink.Link{Cid: c}]
	if !ok {
		return nil, errors.New("No block found")
	}
	return blocks.NewBlockWithCid(data, c)
}

// DumpToCar puts the tree into a car file, with user configured functions
func (tt TestIPLDTree) DumpToCar(out io.Writer, userOnNewCarBlocks ...car.OnNewCarBlockFunc) error {
	ctx := context.Background()
	sc := car.NewSelectiveCar(ctx, tt, []car.Dag{
		{
			Root:     tt.RootNodeLnk.(cidlink.Link).Cid,
			Selector: selectorparse.CommonSelector_ExploreAllRecursively,
		},
	})

	return sc.Write(out, userOnNewCarBlocks...)
}
