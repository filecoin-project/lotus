package blockstore

import (
	"context"
	"io"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/raulk/freecache"
)

type caches struct {
	sync.RWMutex
	caches []*FreecacheCachingBlockstore
}

func (ac *caches) Dirty(cid cid.Cid) {
	k := []byte(cid.Hash())
	ac.RLock()
	for _, c := range ac.caches {
		c.existsCache.Del(k)
	}
	ac.RUnlock()
}

var (
	HasFalse = byte(0)
	HasTrue  = byte(1)

	// Sentinel values for false and true.
	HasFalseVal = []byte{HasFalse}
	HasTrueVal  = []byte{HasTrue}

	// AllCaches holds all caches that have been created, and can be used to
	// declare a CID dirty, in case a
	AllCaches = new(caches)
)

type FreecacheCachingBlockstore struct {
	blockCache  *freecache.Cache
	existsCache *freecache.Cache

	inner LotusBlockstore
}

var _ LotusBlockstore = (*FreecacheCachingBlockstore)(nil)
var _ Viewer = (*FreecacheCachingBlockstore)(nil)

type FreecacheConfig struct {
	Name           string
	BlockCapacity  int
	ExistsCapacity int
}

func WrapFreecacheCache(ctx context.Context, inner LotusBlockstore, config FreecacheConfig) (*FreecacheCachingBlockstore, error) {
	c := &FreecacheCachingBlockstore{
		blockCache:  freecache.NewCache(config.BlockCapacity),
		existsCache: freecache.NewCache(config.ExistsCapacity),
		inner:       inner,
	}

	AllCaches.Lock()
	AllCaches.caches = append(AllCaches.caches, c)
	AllCaches.Unlock()

	go func() {
		blockCacheTag, err := tag.New(ctx, tag.Insert(CacheName, config.Name+"_block_cache"))
		if err != nil {
			log.Warnf("blockstore metrics: failed to instantiate block cache tag: %s", err)
			return
		}
		existsCacheTag, err := tag.New(ctx, tag.Insert(CacheName, config.Name+"_exists_cache"))
		if err != nil {
			log.Warnf("blockstore metrics: failed to instantiate exists cache tag: %s", err)
			return
		}

		recordMetrics := func(ctx context.Context, c *freecache.Cache) {
			stats.Record(ctx,
				CacheMeasures.HitRatio.M(c.HitRate()),
				CacheMeasures.Hits.M(c.HitCount()),
				CacheMeasures.Misses.M(c.MissCount()),
				CacheMeasures.Entries.M(c.EntryCount()),
				CacheMeasures.Updates.M(c.OverwriteCount()),
				CacheMeasures.QueriesServed.M(c.LookupCount()),
				CacheMeasures.Evictions.M(c.EvacuateCount()),
			)
		}
		for {
			select {
			case <-time.After(CacheMetricsEmitInterval):
				recordMetrics(blockCacheTag, c.blockCache)
				recordMetrics(existsCacheTag, c.existsCache)
			case <-ctx.Done():
				return // yield
			}
		}
	}()

	return c, nil
}

// Close clears and closes all caches. It also closes the underlying blockstore,
// if it implements io.Closer.
func (c *FreecacheCachingBlockstore) Close() error {
	c.blockCache.Clear()
	c.existsCache.Clear()
	if closer, ok := c.inner.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (c *FreecacheCachingBlockstore) View(cid cid.Cid, callback func([]byte) error) error {
	k := []byte(cid.Hash())

	// try the cache.
	if val, have, conclusive := c.checkCache(k, true, false); conclusive && have {
		return callback(val)
	} else if conclusive && !have {
		return ErrNotFound
	}

	// fall back to the inner store.
	err := c.inner.View(cid, func(b []byte) error {
		_ = c.existsCache.Set(k, HasTrueVal, 0)
		// set copies the bytes into the cache (it does not retain the byte
		// slice), so this is safe.
		_ = c.blockCache.Set(k, b, 0)
		return callback(b)
	})
	if err == ErrNotFound {
		// inform the has cache that the item does not exist.
		_ = c.existsCache.Set(k, HasFalseVal, 0)
	}
	return err
}

func (c *FreecacheCachingBlockstore) Get(cid cid.Cid) (blocks.Block, error) {
	k := []byte(cid.Hash())

	// try the cache.
	if val, have, conclusive := c.checkCache(k, true, false); conclusive && have {
		return blocks.NewBlockWithCid(val, cid)
	} else if conclusive && !have {
		return nil, ErrNotFound
	}

	// fall back to the inner store.
	res, err := c.inner.Get(cid)
	if err != nil {
		if err == ErrNotFound {
			// inform the has cache that the item does not exist.
			_ = c.existsCache.Set(k, HasFalseVal, 0)
		}
		return res, err
	}
	_ = c.existsCache.Set(k, HasTrueVal, 0)
	_ = c.blockCache.Set(k, res.RawData(), 0)
	return res, err
}

// Peek views a value without affecting the cache counters.
// Peek is currently unused, but may be useful in the future. When the time
// comes, it'll need to be exposed through the abstractions.
func (c *FreecacheCachingBlockstore) Peek(cid cid.Cid, callback func([]byte) error) error {
	k := []byte(cid.Hash())

	// try the cache.
	if val, have, conclusive := c.checkCache(k, true, true); conclusive && have {
		return callback(val)
	} else if conclusive && !have {
		return ErrNotFound
	}

	// fall back to the inner store, with no caching of the returned value.
	return c.inner.View(cid, callback)
}

// Evict evicts a CID form the caches.
// Evict is currently unused, but may be useful in the future. When the time
// comes, it'll need to be exposed through the abstractions.
func (c *FreecacheCachingBlockstore) Evict(cid cid.Cid) {
	k := []byte(cid.Hash())
	c.existsCache.Del(k)
	c.blockCache.Del(k)
}

func (c *FreecacheCachingBlockstore) GetSize(cid cid.Cid) (int, error) {
	k := []byte(cid.Hash())
	// check the has cache.
	if has, err := c.existsCache.Get(k); err == nil && has[0] == HasFalse {
		// we know we don't have the item; short-circuit.
		return -1, ErrNotFound
	}
	res, err := c.inner.GetSize(cid)
	if err != nil {
		if err == ErrNotFound {
			// inform the exists cache that the item does not exist.
			_ = c.existsCache.Set(k, HasFalseVal, 0)
		}
		return res, err
	}
	_ = c.existsCache.Set(k, HasTrueVal, 0)
	return res, err
}

func (c *FreecacheCachingBlockstore) Has(cid cid.Cid) (bool, error) {
	k := []byte(cid.Hash())
	if has, err := c.existsCache.Get(k); err == nil {
		return has[0] == HasTrue, nil
	}
	has, err := c.inner.Has(cid)
	if err != nil {
		return has, err
	}
	if has {
		_ = c.existsCache.Set(k, HasTrueVal, 0)
	} else {
		_ = c.existsCache.Set(k, HasFalseVal, 0)
	}
	return has, err
}

func (c *FreecacheCachingBlockstore) Put(block blocks.Block) error {
	k := []byte(block.Cid().Hash())
	if _, have, conclusive := c.checkCache(k, false, false); conclusive && have {
		return nil
	}
	err := c.inner.Put(block)
	if err != nil {
		return err
	}
	_ = c.blockCache.Set(k, block.RawData(), 0)
	_ = c.existsCache.Set(k, HasTrueVal, 0)
	return err
}

func (c *FreecacheCachingBlockstore) PutMany(blks []blocks.Block) error {
	miss := make([]blocks.Block, 0, len(blks))
	for _, b := range blks {
		k := []byte(b.Cid().Hash())
		if _, have, conclusive := c.checkCache(k, false, false); conclusive && have {
			continue
		}
		miss = append(miss, b)
	}
	if len(miss) == 0 {
		// nothing to add.
		return nil
	}

	err := c.inner.PutMany(miss)
	if err != nil {
		return err
	}
	for _, b := range miss {
		k := []byte(b.Cid().Hash())
		_ = c.blockCache.Set(k, b.RawData(), 0)
		_ = c.existsCache.Set(k, HasTrueVal, 0)
	}
	return err
}

func (c *FreecacheCachingBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return c.inner.AllKeysChan(ctx)
}

func (c *FreecacheCachingBlockstore) DeleteBlock(cid cid.Cid) error {
	k := []byte(cid.Hash())
	err := c.inner.DeleteBlock(cid)
	if err != nil {
		return err
	}
	c.blockCache.Del(k)
	_ = c.existsCache.Set(k, HasFalseVal, 0)
	return err
}

func (c *FreecacheCachingBlockstore) HashOnRead(enabled bool) {
	c.inner.HashOnRead(enabled)
}

// checkCache returns the cached element if we have it. The first boolean
// indicates if we know for sure if the element exists; the second boolean is
// true if the answer is conclusive. If false, the underlying store must be hit.
func (c *FreecacheCachingBlockstore) checkCache(k []byte, wantValue bool, peekOnly bool) (v []byte, have bool, conclusive bool) {
	f := (*freecache.Cache).Get
	if peekOnly {
		f = (*freecache.Cache).Peek
	}
	// check the has cache.
	if has, err := f(c.existsCache, k); err == nil {
		if exists := has[0] == HasTrue; !exists || !wantValue {
			// if we have an entry, and we're only checking for existence, return immediately.
			// if we have an entry, and it's negative, short-circuit.
			return nil, exists, true
		}
	}
	// the has cache was a miss, or we want the value.
	// check the block cache.
	if data, err := f(c.blockCache, k); err == nil {
		if !peekOnly {
			// it's ok to manipulate caches.
			_ = c.existsCache.Set(k, HasTrueVal, 0) // update the exists cache.
		}
		return data, true, true
	}
	return nil, false, false
}
