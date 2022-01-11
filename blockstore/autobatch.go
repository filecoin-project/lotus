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
	bufferedLk     sync.Mutex
	bufferedOrder  []cid.Cid
	bufferedBs     Blockstore
	flushingOrder  []cid.Cid
	flushingBs     Blockstore
	backingBs      Blockstore
	flushReady     chan struct{}
	flushDoWork    chan struct{}
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

func NewAutobatch(ctx context.Context, backingBs Blockstore, bufferSize int) *AutobatchBlockstore {
	bs := &AutobatchBlockstore{
		bufferedBs:  NewMemory(),
		flushingBs:  NewMemory(),
		backingBs:   backingBs,
		bufferSize:  bufferSize,
		flushReady:  make(chan struct{}),
		flushDoWork: make(chan struct{}),
	}

	go bs.flush(ctx)

	return bs
}

func (bs *AutobatchBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	bs.bufferedLk.Lock()
	defer bs.bufferedLk.Unlock()
	if out, err := bs.bufferedBs.Get(ctx, c); err != nil {
		if err != ErrNotFound {
			return nil, err
		}
	} else {
		return out, nil
	}

	if out, err := bs.flushingBs.Get(ctx, c); err != nil {
		if err != ErrNotFound {
			return nil, err
		}
	} else {
		return out, nil
	}

	return bs.backingBs.Get(ctx, c)
}

func (bs *AutobatchBlockstore) Put(ctx context.Context, blk block.Block) error {
	bs.bufferedLk.Lock()
	// TODO this can't move out of the lock, can it?
	if err := bs.bufferedBs.Put(ctx, blk); err != nil {
		return err
	}
	bs.bufferedOrder = append(bs.bufferedOrder, blk.Cid())
	bs.bufferSize += len(blk.RawData())
	if bs.bufferSize >= bs.bufferCapacity {
		// time to flush
		<-bs.flushReady
		bs.flushingBs = bs.bufferedBs
		bs.bufferedBs = NewMemorySync()
		bs.bufferSize = 0
		bs.flushingOrder = bs.bufferedOrder
		bs.bufferedOrder = []cid.Cid{}
		bs.flushDoWork <- struct{}{}
	}
	bs.bufferedLk.Unlock()
	return nil
}

func (bs *AutobatchBlockstore) flush(ctx context.Context) {
	bs.flushReady <- struct{}{}
	for {
		select {
		case <-bs.flushDoWork:
			// todo: can i do more than log error here
			bs.doFlush(ctx)
			bs.flushReady <- struct{}{}
		case <-ctx.Done():
			return
		}
	}
}

func (bs *AutobatchBlockstore) doFlush(ctx context.Context) error {
	blks := make([]block.Block, len(bs.flushingOrder))
	for k, v := range bs.flushingOrder {
		blk, err := bs.Get(ctx, v)
		if err != nil {
			return err
		}

		blks[k] = blk
	}

	return bs.backingBs.PutMany(ctx, blks)
}
