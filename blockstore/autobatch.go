package blockstore

import (
	"context"
	"sync"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// autolog is a logger for the autobatching blockstore. It is subscoped from the
// blockstore logger.
var autolog = log.Named("auto")

type AutobatchBlockstore struct {
	bufferedBlks   []block.Block
	addedCids      map[cid.Cid]struct{}
	bufferedBlksLk sync.Mutex
	flushLk        sync.Mutex
	backingBs      Blockstore
	bufferCapacity int
	bufferSize     int
}

func (bs *AutobatchBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	panic("implement me")
}

func (bs *AutobatchBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	panic("implement me")
}

func (bs *AutobatchBlockstore) GetSize(context.Context, cid.Cid) (int, error) {
	panic("implement me")
}

func (bs *AutobatchBlockstore) PutMany(context.Context, []block.Block) error {
	panic("implement me")
}

func (bs *AutobatchBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	panic("implement me")
}

func (bs *AutobatchBlockstore) HashOnRead(enabled bool) {
	panic("implement me")
}

func (bs *AutobatchBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	panic("implement me")
}

func (bs *AutobatchBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	panic("implement me")
}

func NewAutobatch(ctx context.Context, backingBs Blockstore, bufferCapacity int) *AutobatchBlockstore {
	bs := &AutobatchBlockstore{
		backingBs:      backingBs,
		bufferCapacity: bufferCapacity,
		addedCids:      make(map[cid.Cid]struct{}),
	}

	return bs
}

// May NOT `Get` blocks that have been `Put` into this store
// Only guaranteed to fetch those that were already in the backingBs at creation of this store and those at the most recent `Flush`
func (bs *AutobatchBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	return bs.backingBs.Get(ctx, c)
}

func (bs *AutobatchBlockstore) Put(ctx context.Context, blk block.Block) error {
	bs.bufferedBlksLk.Lock()
	_, ok := bs.addedCids[blk.Cid()]
	if !ok {
		bs.bufferedBlks = append(bs.bufferedBlks, blk)
		bs.addedCids[blk.Cid()] = struct{}{}
		bs.bufferSize += len(blk.RawData())
		if bs.bufferSize >= bs.bufferCapacity {
			// time to flush
			go bs.Flush(ctx)
		}
	}
	bs.bufferedBlksLk.Unlock()
	return nil
}

func (bs *AutobatchBlockstore) Flush(ctx context.Context) {
	bs.flushLk.Lock()
	defer bs.flushLk.Unlock()
	bs.bufferedBlksLk.Lock()
	toFlush := bs.bufferedBlks
	bs.bufferedBlks = []block.Block{}
	bs.bufferedBlksLk.Unlock()
	// error?????
	bs.backingBs.PutMany(ctx, toFlush)
}
