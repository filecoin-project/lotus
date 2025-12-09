package blockstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// BlockstoreCache is a cache for blocks, compatible with lru.Cache; Must be safe for concurrent access
type BlockstoreCache interface {
	Remove(mhString MhString) bool
	Contains(mhString MhString) bool
	Get(mhString MhString) (blocks.Block, bool)
	Add(mhString MhString, block blocks.Block) (evicted bool)
}

type ReadCachedBlockstore struct {
	top   Blockstore
	cache BlockstoreCache
}

type MhString string

func NewReadCachedBlockstore(top Blockstore, cache BlockstoreCache) *ReadCachedBlockstore {
	return &ReadCachedBlockstore{
		top:   top,
		cache: cache,
	}
}

func (c *ReadCachedBlockstore) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	c.cache.Remove(MhString(cid.Hash()))
	return c.top.DeleteBlock(ctx, cid)
}

func (c *ReadCachedBlockstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	if c.cache.Contains(MhString(cid.Hash())) {
		return true, nil
	}

	return c.top.Has(ctx, cid)
}

func (c *ReadCachedBlockstore) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	if out, ok := c.cache.Get(MhString(cid.Hash())); ok {
		return out, nil
	}

	out, err := c.top.Get(ctx, cid)
	if err != nil {
		return nil, err
	}

	c.cache.Add(MhString(cid.Hash()), out)
	return out, nil
}

func (c *ReadCachedBlockstore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	if b, ok := c.cache.Get(MhString(cid.Hash())); ok {
		return len(b.RawData()), nil
	}

	return c.top.GetSize(ctx, cid)
}

func (c *ReadCachedBlockstore) Put(ctx context.Context, block blocks.Block) error {
	c.cache.Add(MhString(block.Cid().Hash()), block)
	return c.top.Put(ctx, block)
}

func (c *ReadCachedBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	for _, b := range blocks {
		c.cache.Add(MhString(b.Cid().Hash()), b)
	}

	return c.top.PutMany(ctx, blocks)
}

func (c *ReadCachedBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return c.top.AllKeysChan(ctx)
}

func (c *ReadCachedBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	return c.top.View(ctx, cid, func(bb []byte) error {
		blk, err := blocks.NewBlockWithCid(bb, cid)
		if err != nil {
			return err
		}

		c.cache.Add(MhString(cid.Hash()), blk)

		return callback(bb)
	})
}

func (c *ReadCachedBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	for _, ci := range cids {
		c.cache.Remove(MhString(ci.Hash()))
	}

	return c.top.DeleteMany(ctx, cids)
}

func (c *ReadCachedBlockstore) Flush(ctx context.Context) error {
	return c.top.Flush(ctx)
}

var _ Blockstore = (*ReadCachedBlockstore)(nil)
