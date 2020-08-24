// blockstore contains all the basic blockstore constructors used by lotus. Any
// blockstores not ultimately constructed out of the building blocks in this
// package may not work properly.
//
//  * This package correctly wraps blockstores with the IdBlockstore. This blockstore:
//    * Filters out all puts for blocks with CIDs using the "identity" hash function.
//    * Extracts inlined blocks from CIDs using the identity hash function and
//      returns them on get/has, ignoring the contents of the blockstore.
//  * In the future, this package may enforce additional restrictions on block
//    sizes, CID validity, etc.
//
// To make auditing for misuse of blockstores tractable, this package re-exports
// parts of the go-ipfs-blockstore package such that no other package needs to
// import it directly.
package blockstore

import (
	"context"

	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

// NewTemporary returns a temporary blockstore.
func NewTemporary() blockstore.Blockstore {
	return NewBlockstore(ds.NewMapDatastore())
}

// NewTemporarySync returns a thread-safe temporary blockstore.
func NewTemporarySync() blockstore.Blockstore {
	return NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
}

// WrapIDStore wraps the underlying blockstore in an "identity" blockstore.
func WrapIDStore(bstore blockstore.Blockstore) blockstore.Blockstore {
	return blockstore.NewIdStore(bstore)
}

// NewBlockstore creates a new blockstore wrapped by the given datastore.
func NewBlockstore(dstore ds.Batching) blockstore.Blockstore {
	return WrapIDStore(blockstore.NewBlockstore(dstore))
}

// Alias so other packages don't have to import go-ipfs-blockstore
type Blockstore = blockstore.Blockstore
type GCBlockstore = blockstore.GCBlockstore
type CacheOpts = blockstore.CacheOpts
type GCLocker = blockstore.GCLocker

var NewGCLocker = blockstore.NewGCLocker
var NewGCBlockstore = blockstore.NewGCBlockstore
var DefaultCacheOpts = blockstore.DefaultCacheOpts
var ErrNotFound = blockstore.ErrNotFound

func CachedBlockstore(ctx context.Context, bs Blockstore, opts CacheOpts) (Blockstore, error) {
	bs, err := blockstore.CachedBlockstore(ctx, bs, opts)
	if err != nil {
		return nil, err
	}
	return WrapIDStore(bs), nil
}
