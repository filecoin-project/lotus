package blockstore

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

type FallbackStore struct {
	Blockstore

	fallbackGetBlock func(context.Context, cid.Cid) (blocks.Block, error)
	lk               sync.RWMutex
}

var _ Blockstore = (*FallbackStore)(nil)

func (fbs *FallbackStore) SetFallback(fg func(context.Context, cid.Cid) (blocks.Block, error)) {
	fbs.lk.Lock()
	defer fbs.lk.Unlock()

	fbs.fallbackGetBlock = fg
}

func (fbs *FallbackStore) getFallback(c cid.Cid) (blocks.Block, error) {
	log.Errorw("fallbackstore: Block not found locally, fetching from the network", "cid", c)
	fbs.lk.RLock()
	defer fbs.lk.RUnlock()

	if fbs.fallbackGetBlock == nil {
		// FallbackStore wasn't configured yet (chainstore/bitswap aren't up yet)
		// Wait for a bit and retry
		fbs.lk.RUnlock()
		time.Sleep(5 * time.Second)
		fbs.lk.RLock()

		if fbs.fallbackGetBlock == nil {
			log.Errorw("fallbackstore: fallbackGetBlock not configured yet")
			return nil, ErrNotFound
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Second)
	defer cancel()

	b, err := fbs.fallbackGetBlock(ctx, c)
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
	case ErrNotFound:
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
	case ErrNotFound:
		b, err := fbs.getFallback(c)
		if err != nil {
			return 0, err
		}
		return len(b.RawData()), nil
	default:
		return sz, err
	}
}
