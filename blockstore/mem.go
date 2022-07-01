package blockstore

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

// NewMemory returns a temporary memory-backed blockstore.
func NewMemory() MemBlockstore {
	return make(MemBlockstore)
}

// MemBlockstore is a terminal blockstore that keeps blocks in memory.
type MemBlockstore map[cid.Cid]blocks.Block

func (m MemBlockstore) DeleteBlock(ctx context.Context, k cid.Cid) error {
	delete(m, k)
	return nil
}

func (m MemBlockstore) DeleteMany(ctx context.Context, ks []cid.Cid) error {
	for _, k := range ks {
		delete(m, k)
	}
	return nil
}

func (m MemBlockstore) Has(ctx context.Context, k cid.Cid) (bool, error) {
	_, ok := m[k]
	return ok, nil
}

func (m MemBlockstore) View(ctx context.Context, k cid.Cid, callback func([]byte) error) error {
	b, ok := m[k]
	if !ok {
		return ipld.ErrNotFound{Cid: k}
	}
	return callback(b.RawData())
}

func (m MemBlockstore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	b, ok := m[k]
	if !ok {
		return nil, ipld.ErrNotFound{Cid: k}
	}
	return b, nil
}

// GetSize returns the CIDs mapped BlockSize
func (m MemBlockstore) GetSize(ctx context.Context, k cid.Cid) (int, error) {
	b, ok := m[k]
	if !ok {
		return 0, ipld.ErrNotFound{Cid: k}
	}
	return len(b.RawData()), nil
}

// Put puts a given block to the underlying datastore
func (m MemBlockstore) Put(ctx context.Context, b blocks.Block) error {
	// Convert to a basic block for safety, but try to reuse the existing
	// block if it's already a basic block.
	k := b.Cid()
	if _, ok := b.(*blocks.BasicBlock); !ok {
		// If we already have the block, abort.
		if _, ok := m[k]; ok {
			return nil
		}
		// the error is only for debugging.
		b, _ = blocks.NewBlockWithCid(b.RawData(), b.Cid())
	}
	m[b.Cid()] = b
	return nil
}

// PutMany puts a slice of blocks at the same time using batching
// capabilities of the underlying datastore whenever possible.
func (m MemBlockstore) PutMany(ctx context.Context, bs []blocks.Block) error {
	for _, b := range bs {
		_ = m.Put(ctx, b) // can't fail
	}
	return nil
}

// AllKeysChan returns a channel from which
// the CIDs in the Blockstore can be read. It should respect
// the given context, closing the channel if it becomes Done.
func (m MemBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	ch := make(chan cid.Cid, len(m))
	for k := range m {
		ch <- k
	}
	close(ch)
	return ch, nil
}

// HashOnRead specifies if every read block should be
// rehashed to make sure it matches its CID.
func (m MemBlockstore) HashOnRead(enabled bool) {
	// no-op
}
