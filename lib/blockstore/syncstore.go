package blockstore

import (
	"context"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

type SyncStore struct {
	mu sync.RWMutex
	bs MemStore // specifically use a memStore to save indirection overhead.
}

func (m *SyncStore) DeleteBlock(k cid.Cid) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.DeleteBlock(k)
}
func (m *SyncStore) Has(k cid.Cid) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.Has(k)
}
func (m *SyncStore) Get(k cid.Cid) (blocks.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.Get(k)
}

// GetSize returns the CIDs mapped BlockSize
func (m *SyncStore) GetSize(k cid.Cid) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.GetSize(k)
}

// Put puts a given block to the underlying datastore
func (m *SyncStore) Put(b blocks.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.Put(b)
}

// PutMany puts a slice of blocks at the same time using batching
// capabilities of the underlying datastore whenever possible.
func (m *SyncStore) PutMany(bs []blocks.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.PutMany(bs)
}

// AllKeysChan returns a channel from which
// the CIDs in the Blockstore can be read. It should respect
// the given context, closing the channel if it becomes Done.
func (m *SyncStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// this blockstore implementation doesn't do any async work.
	return m.bs.AllKeysChan(ctx)
}

// HashOnRead specifies if every read block should be
// rehashed to make sure it matches its CID.
func (m *SyncStore) HashOnRead(enabled bool) {
	// noop
}
