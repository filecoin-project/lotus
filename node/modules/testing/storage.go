package testing

import (
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
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
