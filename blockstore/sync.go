package blockstore

import (
	"context"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// NewMemorySync returns a thread-safe in-memory blockstore.
func NewMemorySync() *SyncBlockstore {
	return &SyncBlockstore{bs: make(MemBlockstore)}
}

// SyncBlockstore is a terminal blockstore that is a synchronized version
// of MemBlockstore.
type SyncBlockstore struct {
	mu sync.RWMutex
	bs MemBlockstore // specifically use a memStore to save indirection overhead.
}

func (*SyncBlockstore) Flush(context.Context) error { return nil }

func (m *SyncBlockstore) DeleteBlock(ctx context.Context, k cid.Cid) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.DeleteBlock(ctx, k)
}

func (m *SyncBlockstore) DeleteMany(ctx context.Context, ks []cid.Cid) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.DeleteMany(ctx, ks)
}

func (m *SyncBlockstore) Has(ctx context.Context, k cid.Cid) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.Has(ctx, k)
}

func (m *SyncBlockstore) View(ctx context.Context, k cid.Cid, callback func([]byte) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.bs.View(ctx, k, callback)
}

func (m *SyncBlockstore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.Get(ctx, k)
}

func (m *SyncBlockstore) GetSize(ctx context.Context, k cid.Cid) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bs.GetSize(ctx, k)
}

func (m *SyncBlockstore) Put(ctx context.Context, b blocks.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.Put(ctx, b)
}

func (m *SyncBlockstore) PutMany(ctx context.Context, bs []blocks.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bs.PutMany(ctx, bs)
}

func (m *SyncBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// this blockstore implementation doesn't do any async work.
	return m.bs.AllKeysChan(ctx)
}
