package blocksync

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	store "github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	ipldselector "github.com/ipld/go-ipld-prime/traversal/selector"
	selectorbuilder "github.com/ipld/go-ipld-prime/traversal/selector/builder"
)

const (

	// AMT selector recursion. An AMT has arity of 8 so this gives allows
	// us to retrieve trees with 8^10 (1,073,741,824) elements.
	amtRecursionDepth = uint32(10)

	// some constants for looking up tuple encoded struct fields
	// field index of Parents field in a block header
	blockIndexParentsField = 3

	// field index of Messages field in a block header
	blockIndexMessagesField = 8

	// field index of AMT node in AMT head
	amtHeadNodeFieldIndex = 2

	// field index of links array AMT node
	amtNodeLinksFieldIndex = 1

	// field index of values array AMT node
	amtNodeValuesFieldIndex = 2

	// maximum depth per traversal
	maxRequestLength = 50
)

var amtSelector selectorbuilder.SelectorSpec

func init() {
	// builer for selectors
	ssb := selectorbuilder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())
	// amt selector -- needed to selector through a messages AMT
	amtSelector = ssb.ExploreIndex(amtHeadNodeFieldIndex,
		ssb.ExploreRecursive(ipldselector.RecursionLimitDepth(int(amtRecursionDepth)),
			ssb.ExploreUnion(
				ssb.ExploreIndex(amtNodeLinksFieldIndex,
					ssb.ExploreAll(ssb.ExploreRecursiveEdge())),
				ssb.ExploreIndex(amtNodeValuesFieldIndex,
					ssb.ExploreAll(ssb.Matcher())))))
}

func selectorForRequest(req *BlockSyncRequest) ipld.Node {
	// builer for selectors
	ssb := selectorbuilder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	bso := ParseBSOptions(req.Options)
	if bso.IncludeMessages {
		return ssb.ExploreRecursive(ipldselector.RecursionLimitDepth(int(req.RequestLength)),
			ssb.ExploreIndex(blockIndexParentsField,
				ssb.ExploreUnion(
					ssb.ExploreAll(
						ssb.ExploreIndex(blockIndexMessagesField,
							ssb.ExploreRange(0, 2, amtSelector),
						)),
					ssb.ExploreIndex(0, ssb.ExploreRecursiveEdge()),
				))).Node()
	}
	return ssb.ExploreRecursive(ipldselector.RecursionLimitDepth(int(req.RequestLength)), ssb.ExploreIndex(blockIndexParentsField,
		ssb.ExploreUnion(
			ssb.ExploreAll(
				ssb.Matcher(),
			),
			ssb.ExploreIndex(0, ssb.ExploreRecursiveEdge()),
		))).Node()
}

func firstTipsetSelector(req *BlockSyncRequest) ipld.Node {
	// builer for selectors
	ssb := selectorbuilder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	bso := ParseBSOptions(req.Options)
	if bso.IncludeMessages {
		return ssb.ExploreIndex(blockIndexMessagesField,
			ssb.ExploreRange(0, 2, amtSelector),
		).Node()
	}
	return ssb.Matcher().Node()

}

func (bs *BlockSync) executeGsyncSelector(ctx context.Context, p peer.ID, root cid.Cid, sel ipld.Node) error {
	_, errs := bs.gsync.Request(ctx, p, cidlink.Link{Cid: root}, sel)

	for err := range errs {
		return xerrors.Errorf("failed to complete graphsync request: %w", err)
	}
	return nil
}

// Fallback for interacting with other non-lotus nodes
func (bs *BlockSync) fetchBlocksGraphSync(ctx context.Context, p peer.ID, req *BlockSyncRequest) (*BlockSyncResponse, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	immediateTsSelector := firstTipsetSelector(req)

	// Do this because we can only request one root at a time
	for _, r := range req.Start {
		if err := bs.executeGsyncSelector(ctx, p, r, immediateTsSelector); err != nil {
			return nil, err
		}
	}

	if req.RequestLength > maxRequestLength {
		req.RequestLength = maxRequestLength
	}

	sel := selectorForRequest(req)

	// execute the selector forreal
	if err := bs.executeGsyncSelector(ctx, p, req.Start[0], sel); err != nil {
		return nil, err
	}

	// Now pull the data we fetched out of the chainstore (where it should now be persisted)
	tempcs := store.NewChainStore(bs.bserv.Blockstore(), datastore.NewMapDatastore(), nil)

	opts := ParseBSOptions(req.Options)
	tsk := types.NewTipSetKey(req.Start...)
	chain, err := collectChainSegment(tempcs, tsk, req.RequestLength, opts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load chain data from chainstore after successful graphsync response (start = %v): %w", req.Start, err)
	}

	return &BlockSyncResponse{Chain: chain}, nil
}
