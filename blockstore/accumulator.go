package blockstore

import (
	"context"
	"sync"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// Accumulator is a type of blockstore that accumulates blocks in memory as they
// are written. An Accumulator otherwise acts as a pass-through to the
// underlying blockstore.
//
// Written blocks can be retrieved later using the GetBlocks method. This is
// useful for testing and debugging purposes. Care should be taken when using
// this feature for long-running applications where many blocks are written as
// there is no automatic eviction mechanism. ClearBlocks can be used to
// manually clear the accumulated blocks.
type Accumulator struct {
	Blockstore

	cids []cid.Cid
	lk   sync.Mutex
}

var (
	_ Blockstore = (*Accumulator)(nil)
	_ Viewer     = (*Accumulator)(nil)
)

// NewAccumulator creates a new Accumulator blockstore that wraps the given
// Blockstore. The Accumulator will accumulate blocks in memory as they are
// written. The blocks can be retrieved later using the GetBlocks method.
func NewAccumulator(bs Blockstore) *Accumulator {
	return &Accumulator{
		Blockstore: bs,
		cids:       make([]cid.Cid, 0),
	}
}

func (acc *Accumulator) Put(ctx context.Context, b block.Block) error {
	acc.lk.Lock()
	acc.cids = append(acc.cids, b.Cid())
	acc.lk.Unlock()
	return acc.Blockstore.Put(ctx, b)
}

func (acc *Accumulator) PutMany(ctx context.Context, blks []block.Block) error {
	acc.lk.Lock()
	for _, b := range blks {
		acc.cids = append(acc.cids, b.Cid())
	}
	acc.lk.Unlock()
	return acc.Blockstore.PutMany(ctx, blks)
}

// GetBlocks returns the blocks that have been put into the accumulator.
// It is safe to call this method concurrently with Put and PutMany.
// The returned slice is the internal slice of blocks, and should be handled
// with care if use of the blockstore is ongoing after this call.
func (acc *Accumulator) GetBlocks(ctx context.Context) ([]block.Block, error) {
	acc.lk.Lock()
	defer acc.lk.Unlock()

	blocks := make([]block.Block, len(acc.cids))
	for i, c := range acc.cids {
		b, err := acc.Blockstore.Get(ctx, c)
		if err != nil {
			return nil, err
		}
		blocks[i] = b
	}
	return blocks, nil
}

// ClearBlocks clears the blocks that have been accumulated in the
// Accumulator. This is useful for testing and debugging purposes.
// It is safe to call this method concurrently with Put and PutMany.
// The blocks are not removed from the underlying blockstore.
func (acc *Accumulator) ClearBlocks() {
	acc.lk.Lock()
	defer acc.lk.Unlock()

	acc.cids = acc.cids[:0]
}
