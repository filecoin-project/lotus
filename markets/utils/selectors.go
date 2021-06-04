package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"

	// must be imported to init() raw-codec support
	_ "github.com/ipld/go-ipld-prime/codec/raw"

	"github.com/ipfs/go-cid"
	mdagipld "github.com/ipfs/go-ipld-format"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
)

func TraverseDag(
	ctx context.Context,
	ds mdagipld.DAGService,
	startFrom cid.Cid,
	optionalSelector ipld.Node,
	visitCallback traversal.AdvVisitFn,
) error {

	// If no selector is given - use *.*
	// See discusion at https://github.com/ipld/go-ipld-prime/issues/171
	if optionalSelector == nil {
		ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
		optionalSelector = ssb.ExploreRecursive(
			selector.RecursionLimitNone(),
			ssb.ExploreUnion(
				ssb.Matcher(),
				ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
			),
		).Node()
	}

	parsedSelector, err := selector.ParseSelector(optionalSelector)
	if err != nil {
		return err
	}

	// not sure what this is for TBH...
	linkContext := ipld.LinkContext{Ctx: ctx}

	// this is what allows us to understand dagpb
	nodePrototypeChooser := dagpb.AddSupportToChooser(
		func(ipld.Link, ipld.LinkContext) (ipld.NodePrototype, error) {
			return basicnode.Prototype.Any, nil
		},
	)

	// this is how we implement GETs
	linkSystem := cidlink.DefaultLinkSystem()
	linkSystem.StorageReadOpener = func(lctx ipld.LinkContext, lnk ipld.Link) (io.Reader, error) {
		cl, isCid := lnk.(cidlink.Link)
		if !isCid {
			return nil, fmt.Errorf("unexpected link type %#v", lnk)
		}

		node, err := ds.Get(lctx.Ctx, cl.Cid)
		if err != nil {
			return nil, err
		}

		return bytes.NewBuffer(node.RawData()), nil
	}

	// this is how we pull the start node out of the DS
	startLink := cidlink.Link{Cid: startFrom}
	startNodePrototype, err := nodePrototypeChooser(startLink, linkContext)
	if err != nil {
		return err
	}
	startNode, err := linkSystem.Load(
		linkContext,
		startLink,
		startNodePrototype,
	)
	if err != nil {
		return err
	}

	// this is the actual execution, invoking the supplied callback
	return traversal.Progress{
		Cfg: &traversal.Config{
			Ctx:                            ctx,
			LinkSystem:                     linkSystem,
			LinkTargetNodePrototypeChooser: nodePrototypeChooser,
		},
	}.WalkAdv(startNode, parsedSelector, visitCallback)
}
