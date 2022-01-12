package blockstore

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

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
	flushCh        chan struct{}
	flushErr       error
	shutdownCh     chan struct{}
	flushCtx       context.Context
	backingBs      Blockstore
	bufferCapacity int
	bufferSize     int
}

func NewAutobatch(ctx context.Context, backingBs Blockstore, bufferCapacity int) *AutobatchBlockstore {
	bs := &AutobatchBlockstore{
		backingBs:      backingBs,
		bufferCapacity: bufferCapacity,
		addedCids:      make(map[cid.Cid]struct{}),
		flushCtx:       ctx,
	}

	go bs.flushWorker()

	return bs
}

func (bs *AutobatchBlockstore) Put(ctx context.Context, blk block.Block) error {
	bs.bufferedBlksLk.Lock()
	_, ok := bs.addedCids[blk.Cid()]
	if !ok {
		bs.bufferedBlks = append(bs.bufferedBlks, blk)
		bs.addedCids[blk.Cid()] = struct{}{}
		bs.bufferSize += len(blk.RawData())
		if bs.bufferSize >= bs.bufferCapacity {
			// signal that a flush is appropriate, may be ignored
			select {
			case bs.flushCh <- struct{}{}:
			default:
				// do nothing
			}
		}
	}
	bs.bufferedBlksLk.Unlock()
	return nil
}

func (bs *AutobatchBlockstore) flushWorker() {
	for {
		select {
		case <-bs.flushCh:
			putErr := bs.doFlush(bs.flushCtx)
			if putErr != nil {
				autolog.Errorf("FLUSH ERRORED: %w", putErr)
				bs.flushErr = xerrors.Errorf("%w, put error: %w", bs.flushErr, putErr)
			}
		case <-bs.shutdownCh:
			return
		}
	}
}

func (bs *AutobatchBlockstore) doFlush(ctx context.Context) error {
	bs.bufferedBlksLk.Lock()
	// We do NOT clear addedCids here, because its purpose is to expedite Puts
	toFlush := bs.bufferedBlks
	bs.bufferedBlks = []block.Block{}
	bs.bufferedBlksLk.Unlock()
	return bs.backingBs.PutMany(ctx, toFlush)
}

func (bs *AutobatchBlockstore) Flush(ctx context.Context) error {
	return bs.doFlush(ctx)
}

func (bs *AutobatchBlockstore) Shutdown(ctx context.Context) error {
	bs.shutdownCh <- struct{}{}
	if bs.flushErr != nil {
		return xerrors.Errorf("flushWorker errored: %w", bs.flushErr)
	}

	// one last flush in case it's needed
	return bs.doFlush(ctx)
}

// May be very slow if the cid queried wasn't in the backingBs at the time of creation of this AutobatchBlockstore
func (bs *AutobatchBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	blk, err := bs.backingBs.Get(ctx, c)
	if err == nil {
		return blk, nil
	}

	if err != ErrNotFound {
		return blk, err
	}

	bs.Flush(ctx)
	return bs.backingBs.Get(ctx, c)
}

func (bs *AutobatchBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	// if we wanted to support this, we would have to:
	// - flush
	// - delete from the backingBs (if present)
	// - remove from addedCids (if present)
	// - if present in addedCids, also walk bufferedBlks and remove if present
	return xerrors.New("deletion is unsupported")
}

func (bs *AutobatchBlockstore) DeleteMany(ctx context.Context, cids []cid.Cid) error {
	// see note in DeleteBlock()
	return xerrors.New("deletion is unsupported")
}

func (bs *AutobatchBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	_, err := bs.Get(ctx, c)
	if err == nil {
		return true, nil
	}
	if err == ErrNotFound {
		return false, nil
	}

	return false, err
}

func (bs *AutobatchBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	blk, err := bs.Get(ctx, c)
	if err != nil {
		return 0, err
	}

	return len(blk.RawData()), nil
}

func (bs *AutobatchBlockstore) PutMany(ctx context.Context, blks []block.Block) error {
	for _, blk := range blks {
		if err := bs.Put(ctx, blk); err != nil {
			return err
		}
	}

	return nil
}

func (bs *AutobatchBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	bs.Flush(ctx)
	return bs.backingBs.AllKeysChan(ctx)
}

func (bs *AutobatchBlockstore) HashOnRead(enabled bool) {
	bs.backingBs.HashOnRead(enabled)
}

func (bs *AutobatchBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	bs.Flush(ctx)
	return bs.backingBs.View(ctx, cid, callback)
}
