package blockstore

import (
	"context"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

type CacheOpts = blockstore.CacheOpts

func DefaultCacheOpts() CacheOpts {
	return CacheOpts{
		HasBloomFilterSize:   0,
		HasBloomFilterHashes: 0,
		HasARCCacheSize:      512 << 10,
	}
}

func CachedBlockstore(ctx context.Context, bs Blockstore, opts CacheOpts) (Blockstore, error) {
	cached, err := blockstore.CachedBlockstore(ctx, bs, opts)
	if err != nil {
		return nil, err
	}
	return WrapIDStore(cached), nil
}
