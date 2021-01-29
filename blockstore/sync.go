package blockstore

import (
	"context"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// NewTemporarySync returns a thread-safe temporary blockstore.
func NewTemporarySync() *SyncBlockstore {
	return &SyncBlockstore{bs: make(MemBlockstore)}
}

// SyncBlockstore is a terminal blockstore that is a synchronized version
// of MemBlockstore.
type SyncBlockstore struct {
	mu sync.RWMutex
	bs MemBlockstore // specifically use a memStore to save indirection overhead.
}

func (m *SyncBlockstore) DeleteBlock(k cid.Cid) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.DeleteBlock(k)
}

func (m *SyncBlockstore) Has(k cid.Cid) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.Has(k)
}

func (m *SyncBlockstore) View(k cid.Cid, callback func([]byte) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.bs.View(k, callback)
}

func (m *SyncBlockstore) Get(k cid.Cid) (blocks.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.Get(k)
}

// GetSize returns the CIDs mapped BlockSize
func (m *SyncBlockstore) GetSize(k cid.Cid) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.GetSize(k)
}

// Put puts a given block to the underlying datastore
func (m *SyncBlockstore) Put(b blocks.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.Put(b)
}

// PutMany puts a slice of blocks at the same time using batching
// capabilities of the underlying datastore whenever possible.
func (m *SyncBlockstore) PutMany(bs []blocks.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.PutMany(bs)
}

// AllKeysChan returns a channel from which
// the CIDs in the Blockstore can be read. It should respect
// the given context, closing the channel if it becomes Done.
func (m *SyncBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// this blockstore implementation doesn't do any async work.
	return m.bs.AllKeysChan(ctx)
}

// HashOnRead specifies if every read block should be
// rehashed to make sure it matches its CID.
func (m *SyncBlockstore) HashOnRead(enabled bool) {
	// noop
}
