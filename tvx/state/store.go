package state

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/blockstore"

	"github.com/filecoin-project/specs-actors/actors/util/adt"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
)

// ProxyingStores implements the ipld store where unknown items are fetched over the node API.
type ProxyingStores struct {
	CBORStore    cbor.IpldStore
	ADTStore     adt.Store
	Datastore    ds.Batching
	Blockstore   blockstore.Blockstore
	BlockService blockservice.BlockService
	Exchange     exchange.Interface
	DAGService   format.DAGService
}

type proxyingBlockstore struct {
	ctx context.Context
	api api.FullNode

	blockstore.Blockstore
}

func (pb *proxyingBlockstore) Get(cid cid.Cid) (blocks.Block, error) {
	if block, err := pb.Blockstore.Get(cid); err == nil {
		return block, err
	}

	// fmt.Printf("fetching cid via rpc: %v\n", cid)
	item, err := pb.api.ChainReadObj(pb.ctx, cid)
	if err != nil {
		return nil, err
	}
	block, err := blocks.NewBlockWithCid(item, cid)
	if err != nil {
		return nil, err
	}

	err = pb.Blockstore.Put(block)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// NewProxyingStore is a blockstore that proxies get requests for unknown CIDs
// to a Filecoin node, via the ChainReadObj RPC.
//
// It also contains all possible stores, services and gadget that IPLD
// requires (quite a handful).
func NewProxyingStore(ctx context.Context, api api.FullNode) *ProxyingStores {
	ds := ds.NewMapDatastore()

	bs := &proxyingBlockstore{
		ctx:        ctx,
		api:        api,
		Blockstore: blockstore.NewBlockstore(ds),
	}

	var (
		cborstore = cbor.NewCborStore(bs)
		offl      = offline.Exchange(bs)
		blkserv   = blockservice.New(bs, offl)
		dserv     = merkledag.NewDAGService(blkserv)
	)

	return &ProxyingStores{
		CBORStore:    cborstore,
		ADTStore:     adt.WrapStore(ctx, cborstore),
		Datastore:    ds,
		Blockstore:   bs,
		Exchange:     offl,
		BlockService: blkserv,
		DAGService:   dserv,
	}
}
