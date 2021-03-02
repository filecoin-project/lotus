package blockstore

import (
	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

var log = logging.Logger("blockstore")

var ErrNotFound = blockstore.ErrNotFound

// Blockstore is the blockstore interface used by Lotus. It is the union
// of the basic go-ipfs blockstore, with other capabilities required by Lotus,
// e.g. View or Sync.
type Blockstore interface {
	blockstore.Blockstore
	blockstore.Viewer
	BatchDeleter
}

// BasicBlockstore is an alias to the original IPFS Blockstore.
type BasicBlockstore = blockstore.Blockstore

type Viewer = blockstore.Viewer

type BatchDeleter interface {
	DeleteMany(cids []cid.Cid) error
}

// WrapIDStore wraps the underlying blockstore in an "identity" blockstore.
// The ID store filters out all puts for blocks with CIDs using the "identity"
// hash function. It also extracts inlined blocks from CIDs using the identity
// hash function and returns them on get/has, ignoring the contents of the
// blockstore.
func WrapIDStore(bstore blockstore.Blockstore) Blockstore {
	if is, ok := bstore.(*idstore); ok {
		// already wrapped
		return is
	}

	if bs, ok := bstore.(Blockstore); ok {
		// we need to wrap our own becase we don't want to neuter the DeleteMany method
		return NewIDStore(bs)
	}

	return NewIDStore(Adapt(bstore))
}

// FromDatastore creates a new blockstore backed by the given datastore.
func FromDatastore(dstore ds.Batching) Blockstore {
	return WrapIDStore(Adapt(blockstore.NewBlockstore(dstore)))
}

type adaptedBlockstore struct {
	blockstore.Blockstore
}

var _ Blockstore = (*adaptedBlockstore)(nil)

func (a *adaptedBlockstore) View(cid cid.Cid, callback func([]byte) error) error {
	blk, err := a.Get(cid)
	if err != nil {
		return err
	}
	return callback(blk.RawData())
}

func (a *adaptedBlockstore) DeleteMany(cids []cid.Cid) error {
	for _, cid := range cids {
		err := a.DeleteBlock(cid)
		if err != nil {
			return err
		}
	}

	return nil
}

// Adapt adapts a standard blockstore to a Lotus blockstore by
// enriching it with the extra methods that Lotus requires (e.g. View, Sync).
//
// View proxies over to Get and calls the callback with the value supplied by Get.
// Sync noops.
func Adapt(bs blockstore.Blockstore) Blockstore {
	if ret, ok := bs.(Blockstore); ok {
		return ret
	}
	return &adaptedBlockstore{bs}
}
