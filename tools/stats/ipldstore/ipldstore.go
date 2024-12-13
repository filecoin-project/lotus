package ipldstore

import (
	"bytes"
	"context"
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"

	"github.com/filecoin-project/lotus/tools/stats/metrics"
)

type ApiIpldStore struct {
	ctx       context.Context
	api       apiIpldStoreApi
	cache     *lru.TwoQueueCache[cid.Cid, []byte]
	cacheSize int
}

type apiIpldStoreApi interface {
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
}

func NewApiIpldStore(ctx context.Context, api apiIpldStoreApi, cacheSize int) (*ApiIpldStore, error) {
	store := &ApiIpldStore{
		ctx:       ctx,
		api:       api,
		cacheSize: cacheSize,
	}

	cache, err := lru.New2Q[cid.Cid, []byte](store.cacheSize)
	if err != nil {
		return nil, err
	}

	store.cache = cache

	return store, nil
}

func (ht *ApiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *ApiIpldStore) read(ctx context.Context, c cid.Cid) ([]byte, error) {
	stats.Record(ctx, metrics.IpldStoreCacheMiss.M(1))
	done := metrics.Timer(ctx, metrics.IpldStoreReadDuration)
	defer done()
	return ht.api.ChainReadObj(ctx, c)
}

func (ht *ApiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	done := metrics.Timer(ctx, metrics.IpldStoreGetDuration)
	defer done()
	defer func() {
		stats.Record(ctx, metrics.IpldStoreCacheSize.M(int64(ht.cacheSize)))
		stats.Record(ctx, metrics.IpldStoreCacheLength.M(int64(ht.cache.Len())))
	}()

	var raw []byte

	if a, ok := ht.cache.Get(c); ok {
		stats.Record(ctx, metrics.IpldStoreCacheHit.M(1))
		raw = a
	} else {
		bs, err := ht.read(ctx, c)
		if err != nil {
			return err
		}

		raw = bs
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}

		ht.cache.Add(c, raw)
		return nil
	}

	return fmt.Errorf("object does not implement CBORUnmarshaler")
}

func (ht *ApiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return cid.Undef, fmt.Errorf("Put is not implemented on ApiIpldStore")
}
