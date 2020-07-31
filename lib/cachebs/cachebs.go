package cachebs

import (
	"context"

	lru "github.com/hashicorp/golang-lru"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("cachebs")

type CacheBS struct {
	cache *lru.ARCCache
	bs    bstore.Blockstore
}

func NewBufferedBstore(base blockstore.Blockstore, size int) *CacheBS {
	c, err := lru.NewARC(size)
	if err != nil {
		panic(err)
	}
	return &CacheBS{
		cache: c,
		bs:    base,
	}
}

var _ (bstore.Blockstore) = &CacheBS{}

func (bs *CacheBS) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return bs.bs.AllKeysChan(ctx)
}

func (bs *CacheBS) DeleteBlock(c cid.Cid) error {
	bs.cache.Remove(c)

	return bs.bs.DeleteBlock(c)
}

func (bs *CacheBS) Get(c cid.Cid) (block.Block, error) {
	v, ok := bs.cache.Get(c)
	if ok {
		return v.(block.Block), nil
	}

	out, err := bs.bs.Get(c)
	if err != nil {
		return nil, err
	}

	bs.cache.Add(c, out)
	return out, nil
}

func (bs *CacheBS) GetSize(c cid.Cid) (int, error) {
	return bs.bs.GetSize(c)
}

func (bs *CacheBS) Put(blk block.Block) error {
	bs.cache.Add(blk.Cid(), blk)

	return bs.bs.Put(blk)
}

func (bs *CacheBS) Has(c cid.Cid) (bool, error) {
	if bs.cache.Contains(c) {
		return true, nil
	}

	return bs.bs.Has(c)
}

func (bs *CacheBS) HashOnRead(hor bool) {
	bs.bs.HashOnRead(hor)
}

func (bs *CacheBS) PutMany(blks []block.Block) error {
	for _, blk := range blks {
		bs.cache.Add(blk.Cid(), blk)
	}
	return bs.bs.PutMany(blks)
}
