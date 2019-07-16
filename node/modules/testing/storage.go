package testing

import (
	"context"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"go.uber.org/fx"
)

func MapBlockstore() blockstore.Blockstore {
	// TODO: proper datastore
	bds := dsync.MutexWrap(datastore.NewMapDatastore())
	bs := blockstore.NewBlockstore(bds)
	return blockstore.NewIdStore(bs)
}

func MapDatastore() datastore.Batching {
	return dsync.MutexWrap(datastore.NewMapDatastore())
}

func MemoryClientDag(lc fx.Lifecycle) ipld.DAGService {
	ibs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	bsvc := blockservice.New(ibs, offline.Exchange(ibs))
	dag := merkledag.NewDAGService(bsvc)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return bsvc.Close()
		},
	})

	return dag
}
