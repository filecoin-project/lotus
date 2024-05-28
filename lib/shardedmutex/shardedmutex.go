package shardedmutex

import (
	"hash/maphash"
	"sync"
)

const cacheline = 64

// padding a mutex to a cacheline improves performance as the cachelines are not contested
// name     old time/op  new time/op  delta
// Locks-8  74.6ns ± 7%  12.3ns ± 2%  -83.54%  (p=0.000 n=20+18)
type paddedMutex struct {
	mt sync.Mutex
	_  [cacheline - 8]uint8
}

type ShardedMutex struct {
	shards []paddedMutex
}

// New creates a new ShardedMutex with N shards
func New(nShards int) ShardedMutex {
	if nShards < 1 {
		panic("n_shards cannot be less than 1")
	}
	return ShardedMutex{
		shards: make([]paddedMutex, nShards),
	}
}

func (sm ShardedMutex) Shards() int {
	return len(sm.shards)
}

func (sm ShardedMutex) Lock(shard int) {
	sm.shards[shard].mt.Lock()
}

func (sm ShardedMutex) Unlock(shard int) {
	sm.shards[shard].mt.Unlock()
}

func (sm ShardedMutex) GetLock(shard int) sync.Locker {
	return &sm.shards[shard].mt
}

type ShardedMutexFor[K any] struct {
	inner ShardedMutex

	hasher func(maphash.Seed, K) uint64
	seed   maphash.Seed
}

func NewFor[K any](hasher func(maphash.Seed, K) uint64, nShards int) ShardedMutexFor[K] {
	return ShardedMutexFor[K]{
		inner:  New(nShards),
		hasher: hasher,
		seed:   maphash.MakeSeed(),
	}
}

func (sm ShardedMutexFor[K]) shardFor(key K) int {
	return int(sm.hasher(sm.seed, key) % uint64(len(sm.inner.shards)))
}

func (sm ShardedMutexFor[K]) Lock(key K) {
	sm.inner.Lock(sm.shardFor(key))
}
func (sm ShardedMutexFor[K]) Unlock(key K) {
	sm.inner.Unlock(sm.shardFor(key))
}
func (sm ShardedMutexFor[K]) GetLock(key K) sync.Locker {
	return sm.inner.GetLock(sm.shardFor(key))
}
