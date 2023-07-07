package blockstore

import (
	"context"
	"time"

	blockstore "github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("blockstore")

// Blockstore is the blockstore interface used by Lotus. It is the union
// of the basic go-ipfs blockstore, with other capabilities required by Lotus,
// e.g. View or Sync.
type Blockstore interface {
	blockstore.Blockstore
	blockstore.Viewer
	BatchDeleter
	Flusher
}

// BasicBlockstore is an alias to the original IPFS Blockstore.
type BasicBlockstore = blockstore.Blockstore

type Viewer = blockstore.Viewer

type Flusher interface {
	Flush(context.Context) error
}

type BatchDeleter interface {
	DeleteMany(ctx context.Context, cids []cid.Cid) error
}

// BlockstoreIterator is a trait for efficient iteration
type BlockstoreIterator interface {
	ForEachKey(func(cid.Cid) error) error
}

// BlockstoreGC is a trait for blockstores that support online garbage collection
type BlockstoreGC interface {
	CollectGarbage(ctx context.Context, options ...BlockstoreGCOption) error
}

// BlockstoreGCOnce is a trait for a blockstore that supports incremental online garbage collection
type BlockstoreGCOnce interface {
	GCOnce(ctx context.Context, options ...BlockstoreGCOption) error
}

// BlockstoreGCOption is a functional interface for controlling blockstore GC options
type BlockstoreGCOption = func(*BlockstoreGCOptions) error

// BlockstoreGCOptions is a struct with GC options
type BlockstoreGCOptions struct {
	FullGC bool
	// fraction of garbage in badger vlog before its worth processing in online GC
	Threshold float64
	// how often to call the check function
	CheckFreq time.Duration
	// function to call periodically to pause or early terminate GC
	Check func() error
}

func WithFullGC(fullgc bool) BlockstoreGCOption {
	return func(opts *BlockstoreGCOptions) error {
		opts.FullGC = fullgc
		return nil
	}
}

func WithThreshold(threshold float64) BlockstoreGCOption {
	return func(opts *BlockstoreGCOptions) error {
		opts.Threshold = threshold
		return nil
	}
}

func WithCheckFreq(f time.Duration) BlockstoreGCOption {
	return func(opts *BlockstoreGCOptions) error {
		opts.CheckFreq = f
		return nil
	}
}

func WithCheck(check func() error) BlockstoreGCOption {
	return func(opts *BlockstoreGCOptions) error {
		opts.Check = check
		return nil
	}
}

// BlockstoreSize is a trait for on-disk blockstores that can report their size
type BlockstoreSize interface {
	Size() (int64, error)
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
		// we need to wrap our own because we don't want to neuter the DeleteMany method
		// the underlying blockstore has implemented an (efficient) DeleteMany
		return NewIDStore(bs)
	}

	// The underlying blockstore does not implement DeleteMany, so we need to shim it.
	// This is less efficient as it'll iterate and perform single deletes.
	return NewIDStore(Adapt(bstore))
}

// FromDatastore creates a new blockstore backed by the given datastore.
func FromDatastore(dstore ds.Batching) Blockstore {
	return WrapIDStore(blockstore.NewBlockstore(dstore))
}

type adaptedBlockstore struct {
	blockstore.Blockstore
}

var _ Blockstore = (*adaptedBlockstore)(nil)

func (a *adaptedBlockstore) Flush(ctx context.Context) error {
	if flusher, canFlush := a.Blockstore.(Flusher); canFlush {
		return flusher.Flush(ctx)
	}
	return nil
}

func (a *adaptedBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	blk, err := a.Get(ctx, cid)
	if err != nil {
		return err
	}
	return callback(blk.RawData())
}

func (a *adaptedBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	for _, cid := range cids {
		err := a.DeleteBlock(ctx, cid)
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
