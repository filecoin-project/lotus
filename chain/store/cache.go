package store

import (
	"context"
	"fmt"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/blockstore"
)

type CachingBlockstore struct {
	cache  *lru.ARCCache
	blocks blockstore.Blockstore
	reads  int64 // updated atomically
	hits   int64 // updated atomically
	bytes  int64 // updated atomically
}

func NewCachingBlockstore(blocks blockstore.Blockstore, cacheSize int) (*CachingBlockstore, error) {
	cache, err := lru.NewARC(cacheSize)
	if err != nil {
		return nil, fmt.Errorf("new arc: %w", err)
	}

	return &CachingBlockstore{
		cache:  cache,
		blocks: blocks,
	}, nil
}

func (cs *CachingBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	return cs.blocks.DeleteBlock(ctx, c)
}

func (cs *CachingBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	return cs.blocks.GetSize(ctx, c)
}

func (cs *CachingBlockstore) Put(ctx context.Context, blk blocks.Block) error {
	return cs.blocks.Put(ctx, blk)
}

func (cs *CachingBlockstore) PutMany(ctx context.Context, blks []blocks.Block) error {
	return cs.blocks.PutMany(ctx, blks)
}

func (cs *CachingBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return cs.blocks.AllKeysChan(ctx)
}

func (cs *CachingBlockstore) HashOnRead(enabled bool) {
	cs.blocks.HashOnRead(enabled)
}

func (cs *CachingBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	return cs.blocks.DeleteMany(ctx, cids)
}

func (cs *CachingBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	reads := atomic.AddInt64(&cs.reads, 1)
	if reads%1000000 == 0 {
		hits := atomic.LoadInt64(&cs.hits)
		by := atomic.LoadInt64(&cs.bytes)
		log.Infow("CachingBlockstore stats", "reads", reads, "cache_len", cs.cache.Len(), "hit_rate", float64(hits)/float64(reads), "bytes_read", by)
	}

	v, hit := cs.cache.Get(c)
	if hit {
		atomic.AddInt64(&cs.hits, 1)
		return v.(blocks.Block), nil
	}

	blk, err := cs.blocks.Get(ctx, c)
	if err != nil {
		return nil, err
	}

	atomic.AddInt64(&cs.bytes, int64(len(blk.RawData())))
	cs.cache.Add(c, blk)
	return blk, err
}

func (cs *CachingBlockstore) View(ctx context.Context, c cid.Cid, callback func([]byte) error) error {
	reads := atomic.AddInt64(&cs.reads, 1)
	if reads%1000000 == 0 {
		hits := atomic.LoadInt64(&cs.hits)
		by := atomic.LoadInt64(&cs.bytes)
		log.Infow("CachingBlockstore stats", "reads", reads, "cache_len", cs.cache.Len(), "hit_rate", float64(hits)/float64(reads), "bytes_read", by)
	}
	v, hit := cs.cache.Get(c)
	if hit {
		atomic.AddInt64(&cs.hits, 1)
		return callback(v.(blocks.Block).RawData())
	}

	blk, err := cs.blocks.Get(ctx, c)
	if err != nil {
		return err
	}

	atomic.AddInt64(&cs.bytes, int64(len(blk.RawData())))
	cs.cache.Add(c, blk)
	return callback(blk.RawData())
}

func (cs *CachingBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	atomic.AddInt64(&cs.reads, 1)
	// Safe to query cache since blockstore never deletes
	if cs.cache.Contains(c) {
		return true, nil
	}

	return cs.blocks.Has(ctx, c)
}
