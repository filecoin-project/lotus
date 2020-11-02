package blockstore

import (
	"context"
	"sync"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// NewTempBlocktore returns a thread-safe temporary memory-backed blockstore
func NewTempBlocktore() XBlockstore {
	return &tempBS{
		store: make(map[cid.Cid]blocks.Block, 512)}
}

// FIXME - temporary shim to make the test vectors happy
var NewTemporary = NewTempBlocktore

type tempBS struct {
	mu    sync.RWMutex
	store map[cid.Cid]blocks.Block
}

func (tbs *tempBS) DeleteBlock(k cid.Cid) error {
	tbs.mu.Lock()
	defer tbs.mu.Unlock()

	delete(tbs.store, k)
	return nil
}

func (tbs *tempBS) Has(k cid.Cid) (bool, error) {
	tbs.mu.RLock()
	defer tbs.mu.RUnlock()

	_, ok := tbs.store[k]
	return ok, nil
}

func (tbs *tempBS) Get(k cid.Cid) (blocks.Block, error) {
	tbs.mu.RLock()
	defer tbs.mu.RUnlock()

	b, ok := tbs.store[k]
	if !ok {
		return nil, ErrNotFound
	}
	return b, nil
}

func (tbs *tempBS) GetSize(k cid.Cid) (int, error) {
	tbs.mu.RLock()
	defer tbs.mu.RUnlock()

	b, ok := tbs.store[k]
	if !ok {
		return -1, ErrNotFound
	}
	return len(b.RawData()), nil
}

func (tbs *tempBS) Put(b blocks.Block) error {
	tbs.mu.Lock()
	defer tbs.mu.Unlock()

	// Convert to a basic block for safety, but try to reuse the existing
	// block if it's already a basic block.
	if _, ok := b.(*blocks.BasicBlock); !ok {
		// If we already have the block, abort.
		if _, ok := tbs.store[b.Cid()]; ok {
			return nil
		}

		// the error is only for debugging.
		b, _ = blocks.NewBlockWithCid(b.RawData(), b.Cid())
	}

	tbs.store[b.Cid()] = b
	return nil
}

func (tbs *tempBS) PutMany(bs []blocks.Block) error {
	for _, b := range bs {
		tbs.Put(b) // nolint:errcheck
	}
	return nil
}

func (tbs *tempBS) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	tbs.mu.RLock()
	defer tbs.mu.RUnlock()

	// this blockstore implementation doesn't do any async work.
	ch := make(chan cid.Cid, len(tbs.store))
	for k := range tbs.store {
		ch <- k
	}

	close(ch)
	return ch, nil
}

func (*tempBS) HashOnRead(bool) {
	// noop
}
