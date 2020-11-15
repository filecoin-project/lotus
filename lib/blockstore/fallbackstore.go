package blockstore

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("blockstore")

// UnwrapFallbackStore takes a blockstore, and returns the underlying blockstore
// if it was a FallbackStore. Otherwise, it just returns the supplied store
// unmodified.
func UnwrapFallbackStore(bs blockstore.Blockstore) (blockstore.Blockstore, bool) {
	if fbs, ok := bs.(*FallbackStore); ok {
		return fbs.Blockstore, true
	}
	return bs, false
}

// FallbackStore is a read-through store that queries another (potentially
// remote) source if the block is not found locally. If the block is found
// during the fallback, it stores it in the local store.
type FallbackStore struct {
	blockstore.Blockstore

	lk     sync.RWMutex
	getter func(context.Context, cid.Cid) (blocks.Block, error)
}

func (fbs *FallbackStore) SetFallback(fg func(context.Context, cid.Cid) (blocks.Block, error)) {
	fbs.lk.Lock()
	defer fbs.lk.Unlock()

	fbs.getter = fg
}

func (fbs *FallbackStore) getFallback(c cid.Cid) (blocks.Block, error) {
	log.Warnf("fallbackstore: block not found locally, fetching from the network; cid: %s", c)
	fbs.lk.RLock()
	defer fbs.lk.RUnlock()

	if fbs.getter == nil {
		// FallbackStore wasn't configured yet (chainstore/bitswap aren't up yet)
		// Wait for a bit and retry
		fbs.lk.RUnlock()
		time.Sleep(5 * time.Second)
		fbs.lk.RLock()

		if fbs.getter == nil {
			log.Errorw("fallbackstore: getter not configured yet")
			return nil, blockstore.ErrNotFound
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Second)
	defer cancel()

	b, err := fbs.getter(ctx, c)
	if err != nil {
		return nil, err
	}

	// chain bitswap puts blocks in temp blockstore which is cleaned up
	// every few min (to drop any messages we fetched but don't want)
	// in this case we want to keep this block around
	if err := fbs.Put(b); err != nil {
		return nil, xerrors.Errorf("persisting fallback-fetched block: %w", err)
	}
	return b, nil
}

func (fbs *FallbackStore) Get(c cid.Cid) (blocks.Block, error) {
	b, err := fbs.Blockstore.Get(c)
	switch err {
	case nil:
		return b, nil
	case blockstore.ErrNotFound:
		return fbs.getFallback(c)
	default:
		return b, err
	}
}

func (fbs *FallbackStore) GetSize(c cid.Cid) (int, error) {
	sz, err := fbs.Blockstore.GetSize(c)
	switch err {
	case nil:
		return sz, nil
	case blockstore.ErrNotFound:
		b, err := fbs.getFallback(c)
		if err != nil {
			return 0, err
		}
		return len(b.RawData()), nil
	default:
		return sz, err
	}
}

var _ blockstore.Blockstore = &FallbackStore{}
