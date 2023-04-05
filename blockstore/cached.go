package blockstore

import (
	"context"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-cid"
	block "github.com/ipfs/go-libipfs/blocks"
)

type CachedBlockstore struct {
	write Blockstore
	cache *lru.ARCCache[cid.Cid, block.Block]
}

var CacheBstoreSize = (4 << 30) / 16000 // 4GB with average block size of 16KB

func WithCache(base Blockstore) *CachedBlockstore {
	c, _ := lru.NewARC[cid.Cid, block.Block](CacheBstoreSize)

	bs := &CachedBlockstore{
		write: base,

		cache: c,
	}
	return bs
}

var (
	_ Blockstore = (*CachedBlockstore)(nil)
	_ Viewer     = (*CachedBlockstore)(nil)
)

func (bs *CachedBlockstore) Flush(ctx context.Context) error { return bs.write.Flush(ctx) }

func (bs *CachedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return bs.write.AllKeysChan(ctx)
}

func (bs *CachedBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	bs.cache.Remove(c)
	return bs.write.DeleteBlock(ctx, c)
}

func (bs *CachedBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	for _, c := range cids {
		bs.cache.Remove(c)
	}
	return bs.write.DeleteMany(ctx, cids)
}

func (bs *CachedBlockstore) View(ctx context.Context, c cid.Cid, callback func([]byte) error) error {
	if blk, ok := bs.cache.Get(c); ok {
		return callback(blk.RawData())
	}

	return bs.write.View(ctx, c, func(bytes []byte) error {
		blk, err := block.NewBlockWithCid(bytes, c)
		if err != nil {
			return err
		}
		bs.cache.Add(c, blk)

		return callback(bytes)
	})
}

func (bs *CachedBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	if blk, ok := bs.cache.Get(c); ok {
		return blk, nil
	}

	return bs.write.Get(ctx, c)
}

func (bs *CachedBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	b, err := bs.Get(ctx, c)
	if err != nil {
		return 0, err
	}

	return len(b.RawData()), nil
}

func (bs *CachedBlockstore) Put(ctx context.Context, blk block.Block) error {
	bs.cache.Add(blk.Cid(), blk)

	return bs.write.Put(ctx, blk)
}

func (bs *CachedBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if bs.cache.Contains(c) {
		return true, nil
	}

	return bs.write.Has(ctx, c)
}

func (bs *CachedBlockstore) HashOnRead(hor bool) {
	bs.write.HashOnRead(hor)
}

func (bs *CachedBlockstore) PutMany(ctx context.Context, blks []block.Block) error {
	for _, blk := range blks {
		bs.cache.Add(blk.Cid(), blk)
	}
	return bs.write.PutMany(ctx, blks)
}
