package filter

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"lukechampine.com/blake3"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

// ActorResolver resolves an address to its actor ID at a given TipSet, or the latest TipSet if nil.
type ActorResolver func(context.Context, address.Address, *types.TipSet) (abi.ActorID, error)

type cachedActorResolver struct {
	delegate       ActorResolver
	cache          *expirable.LRU[string, cachedActorResolution]
	cacheNilTipSet bool
}

type cachedActorResolution struct {
	result abi.ActorID
	err    error
}

var cacheHasherPool = sync.Pool{
	New: func() any {
		return blake3.New(32, nil)
	},
}

func NewCachedActorResolver(delegate ActorResolver, size int, expiry time.Duration, cacheNilTipSet bool) ActorResolver {

	resolver := &cachedActorResolver{
		delegate:       delegate,
		cache:          expirable.NewLRU[string, cachedActorResolution](size, nil, expiry),
		cacheNilTipSet: cacheNilTipSet,
	}
	return resolver.resolve
}

func (c *cachedActorResolver) resolve(ctx context.Context, addr address.Address, ts *types.TipSet) (abi.ActorID, error) {
	if ts == nil && !c.cacheNilTipSet {
		return c.delegate(ctx, addr, ts)
	}
	key := c.newCacheKey(addr, ts)
	resolution, found := c.cache.Get(key)
	if !found {
		resolution.result, resolution.err = c.delegate(ctx, addr, ts)
		c.cache.Add(key, resolution)
	}
	return resolution.result, resolution.err
}

func (c *cachedActorResolver) newCacheKey(addr address.Address, ts *types.TipSet) string {
	// TODO: How much do we care about efficiency?
	//       If not much, use tipset key directly in exchange for larger memory footprint.
	hasher := cacheHasherPool.Get().(*blake3.Hasher)
	defer func() {
		hasher.Reset()
		cacheHasherPool.Put(hasher)
	}()
	if ts != nil {
		_, _ = hasher.Write(ts.Key().Bytes())
	}
	_, _ = hasher.Write(addr.Bytes())
	return string(hasher.Sum(nil))
}
