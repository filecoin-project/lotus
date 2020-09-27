package state

import (
	"context"
	"log"
	"sync"

	"github.com/fatih/color"
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

// Stores is a collection of the different stores and services that are needed
// to deal with the data layer of Filecoin, conveniently interlinked with one
// another.
type Stores struct {
	CBORStore    cbor.IpldStore
	ADTStore     adt.Store
	Datastore    ds.Batching
	Blockstore   blockstore.Blockstore
	BlockService blockservice.BlockService
	Exchange     exchange.Interface
	DAGService   format.DAGService
}

func newStores(ctx context.Context, ds ds.Batching, bs blockstore.Blockstore) *Stores {
	var (
		cborstore = cbor.NewCborStore(bs)
		offl      = offline.Exchange(bs)
		blkserv   = blockservice.New(bs, offl)
		dserv     = merkledag.NewDAGService(blkserv)
	)

	return &Stores{
		CBORStore:    cborstore,
		ADTStore:     adt.WrapStore(ctx, cborstore),
		Datastore:    ds,
		Blockstore:   bs,
		Exchange:     offl,
		BlockService: blkserv,
		DAGService:   dserv,
	}
}

type proxyingBlockstore struct {
	ctx context.Context
	api api.FullNode

	lk      sync.RWMutex
	tracing bool
	traced  map[cid.Cid]struct{}

	blockstore.Blockstore
}

type TracingBlockstore interface {
	StartTracing()
	FinishTracing() map[cid.Cid]struct{}
}

var _ TracingBlockstore = (*proxyingBlockstore)(nil)

// StartTracing starts tracing the CIDs that are effectively fetched during the
// processing of a message.
func (pb *proxyingBlockstore) StartTracing() {
	pb.lk.Lock()
	pb.tracing = true
	pb.traced = map[cid.Cid]struct{}{}
	pb.lk.Unlock()
}

// FinishTracing finishes tracing accessed CIDs, and returns a map of the
// CIDs that were traced.
func (pb *proxyingBlockstore) FinishTracing() map[cid.Cid]struct{} {
	pb.lk.Lock()
	ret := pb.traced
	pb.tracing = false
	pb.traced = map[cid.Cid]struct{}{}
	pb.lk.Unlock()
	return ret
}

func (pb *proxyingBlockstore) Get(cid cid.Cid) (blocks.Block, error) {
	pb.lk.RLock()
	if pb.tracing {
		pb.traced[cid] = struct{}{}
	}
	pb.lk.RUnlock()

	if block, err := pb.Blockstore.Get(cid); err == nil {
		return block, err
	}

	log.Println(color.CyanString("fetching cid via rpc: %v", cid))
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

// NewProxyingStores is a Stores that proxies get requests for unknown CIDs
// to a Filecoin node, via the ChainReadObj RPC.
func NewProxyingStores(ctx context.Context, api api.FullNode) *Stores {
	ds := ds.NewMapDatastore()

	bs := &proxyingBlockstore{
		ctx:        ctx,
		api:        api,
		Blockstore: blockstore.NewBlockstore(ds),
	}

	return newStores(ctx, ds, bs)
}
