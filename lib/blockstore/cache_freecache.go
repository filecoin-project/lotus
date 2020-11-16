package blockstore

import (
	"context"
	"errors"
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

	inner Blockstore
}

var _ Blockstore = (*FreecacheCachingBlockstore)(nil)
var _ Viewer = (*FreecacheCachingBlockstore)(nil)

type FreecacheConfig struct {
	Name           string
	BlockCapacity  int
	ExistsCapacity int
}

func WrapFreecacheCache(ctx context.Context, inner Blockstore, config FreecacheConfig) (*FreecacheCachingBlockstore, error) {
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
	if have, err, conclusive := c.cachedValFn(k, callback, false); conclusive && have {
		return err
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
	if val, have, conclusive := c.cachedVal(k); conclusive && have {
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
	if have, err, conclusive := c.cachedValFn(k, callback, true); conclusive && have {
		return err
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
	// try the cache.
	if val, have, conclusive := c.cachedVal(k); conclusive && have {
		return len(val), nil
	} else if conclusive && !have {
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
	if have, conclusive := c.cachedExists(k); conclusive {
		return have, nil
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
	if have, conclusive := c.cachedExists(k); conclusive && have {
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
		if have, conclusive := c.cachedExists(k); conclusive && have {
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

var errNotExists = errors.New("block doesn't exist")

var existsFn = func(v []byte) error {
	if v[0] == HasTrue {
		return nil
	}
	return errNotExists
}

// cachedExists checks if a value is in the exists cache.
func (c *FreecacheCachingBlockstore) cachedExists(k []byte) (have bool, conclusive bool) {
	if err := c.existsCache.GetFn(k, existsFn); err != freecache.ErrNotFound {
		return err == nil, true
	}
	return false, false
}

// cachedValFn attempts to retrieve a value from cache. It checks the exists
// cache first and short-circuits if a negative is stored. Else, it checks the
// block cache.
func (c *FreecacheCachingBlockstore) cachedValFn(k []byte, valFn func([]byte) error, peek bool) (have bool, err error, conclusive bool) { //nolint:golint
	f := (*freecache.Cache).GetFn
	if peek {
		f = (*freecache.Cache).PeekFn
	}
	// check the exists cache.
	if err := f(c.existsCache, k, existsFn); err == errNotExists {
		return false, nil, true
	}
	if err := f(c.blockCache, k, valFn); err != freecache.ErrNotFound {
		return true, err, true
	}
	return false, nil, false
}

// cachedVal attempts to retrieve a value from cache. It checks the exists cache
// first and short-circuits if a negative is stored. Else, it checks the block
// cache.
func (c *FreecacheCachingBlockstore) cachedVal(k []byte) (val []byte, have bool, conclusive bool) {
	// check the exists cache.
	if err := c.existsCache.GetFn(k, existsFn); err == errNotExists {
		return nil, false, true
	}
	if val, err := c.blockCache.Get(k); err != freecache.ErrNotFound {
		return val, true, true
	}
	return nil, false, false
}
