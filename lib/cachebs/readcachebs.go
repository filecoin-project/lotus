package cachebs

import (
	"context"

	"github.com/filecoin-project/lotus/lib/blockstore"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

type ReadCacheBS struct {
	cache bstore.MemStore
	bs    bstore.Blockstore
}

func NewReadCacheBS(base bstore.Blockstore) bstore.Blockstore {
	cache := bstore.NewTemporary()
	// Wrap this in an ID blockstore to avoid caching blocks inlined into
	// CIDs.
	return bstore.WrapIDStore(&ReadCacheBS{
		cache: cache,
		bs:    base,
	})
}

var _ (bstore.Blockstore) = &CacheBS{}

func (bs *ReadCacheBS) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return bs.bs.AllKeysChan(ctx)
}

func (bs *ReadCacheBS) DeleteBlock(c cid.Cid) error {
	_ = bs.cache.DeleteBlock(c)

	return bs.bs.DeleteBlock(c)
}

func (bs *ReadCacheBS) Get(c cid.Cid) (block.Block, error) {
	b, err := bs.cache.Get(c)
	if err == blockstore.ErrNotFound {
		b, err = bs.bs.Get(c)
		if err != nil {
			return nil, err
		}
		_ = bs.cache.Put(b)
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (bs *ReadCacheBS) GetSize(c cid.Cid) (int, error) {
	size, err := bs.cache.GetSize(c)
	if err == blockstore.ErrNotFound {
		size, err = bs.bs.GetSize(c)
	}
	return size, err
}

func (bs *ReadCacheBS) Put(blk block.Block) error {
	if has, err := bs.cache.Has(blk.Cid()); err == nil && has {
		return nil
	}
	_ = bs.cache.Put(blk)
	return bs.bs.Put(blk)
}

func (bs *ReadCacheBS) Has(c cid.Cid) (bool, error) {
	if has, err := bs.cache.Has(c); err == nil && has {
		return true, nil
	}
	return bs.bs.Has(c)
}

func (bs *ReadCacheBS) HashOnRead(hor bool) {
	bs.bs.HashOnRead(hor)
}

func (bs *ReadCacheBS) PutMany(blks []block.Block) error {
	newBlks := make([]block.Block, 0, len(blks))
	for _, blk := range blks {
		if has, err := bs.cache.Has(blk.Cid()); err == nil && has {
			continue
		}
		newBlks = append(newBlks, blk)
	}
	_ = bs.cache.PutMany(newBlks)
	return bs.bs.PutMany(newBlks)
}
