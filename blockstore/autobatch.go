package blockstore

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

// autolog is a logger for the autobatching blockstore. It is subscoped from the
// blockstore logger.
var autolog = log.Named("auto")

type AutobatchBlockstore struct {
	// TODO: drop if memory consumption is too high
	addedCids map[cid.Cid]struct{}

	bufferedLk          sync.Mutex
	bufferedBlksOrdered []block.Block
	bufferedBlksMap     map[cid.Cid]block.Block

	flushingLk      sync.Mutex
	flushingBlksMap map[cid.Cid]block.Block

	flushCh         chan struct{}
	flushErr        error
	flushRetryDelay time.Duration
	flushCtx        context.Context
	shutdownCh      chan struct{}

	backingBs Blockstore

	bufferCapacity int
	bufferSize     int
}

func NewAutobatch(ctx context.Context, backingBs Blockstore, bufferCapacity int) *AutobatchBlockstore {
	bs := &AutobatchBlockstore{
		addedCids:       make(map[cid.Cid]struct{}),
		backingBs:       backingBs,
		bufferCapacity:  bufferCapacity,
		bufferedBlksMap: make(map[cid.Cid]block.Block),
		flushingBlksMap: make(map[cid.Cid]block.Block),
		flushCtx:        ctx,
		flushCh:         make(chan struct{}, 1),
		// could be made configable
		flushRetryDelay: time.Second * 5,
	}

	go bs.flushWorker()

	return bs
}

func (bs *AutobatchBlockstore) Put(ctx context.Context, blk block.Block) error {
	bs.bufferedLk.Lock()
	_, ok := bs.addedCids[blk.Cid()]
	if !ok {
		bs.addedCids[blk.Cid()] = struct{}{}
		bs.bufferedBlksOrdered = append(bs.bufferedBlksOrdered, blk)
		bs.bufferedBlksMap[blk.Cid()] = blk
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
	bs.bufferedLk.Unlock()
	return nil
}

func (bs *AutobatchBlockstore) flushWorker() {
	for {
		select {
		case <-bs.flushCh:
			putErr := bs.doFlush(bs.flushCtx)
			for putErr != nil {
				select {
				case <-bs.shutdownCh:
					bs.flushErr = putErr
					return
				default:
					autolog.Errorf("FLUSH ERRORED: %w, retrying in %v", putErr, bs.flushRetryDelay)
					time.Sleep(bs.flushRetryDelay)
					putErr = bs.doFlush(bs.flushCtx)
				}
			}
		case <-bs.shutdownCh:
			return
		}
	}
}

func (bs *AutobatchBlockstore) doFlush(ctx context.Context) error {
	bs.bufferedLk.Lock()
	bs.flushingLk.Lock()
	// We do NOT clear addedCids here, because its purpose is to expedite Puts
	flushingBlksOrdered := bs.bufferedBlksOrdered
	bs.flushingBlksMap = bs.bufferedBlksMap
	bs.bufferedBlksOrdered = []block.Block{}
	bs.bufferedBlksMap = make(map[cid.Cid]block.Block)
	bs.bufferedLk.Unlock()
	bs.flushingLk.Unlock()
	return bs.backingBs.PutMany(ctx, flushingBlksOrdered)
}

func (bs *AutobatchBlockstore) Shutdown(ctx context.Context) error {
	// request one last flush of the worker
	bs.flushCh <- struct{}{}
	// shutdown the flush worker
	bs.shutdownCh <- struct{}{}
	// if it ever errored, this method fails
	if bs.flushErr != nil {
		return xerrors.Errorf("flushWorker errored: %w", bs.flushErr)
	}

	// one last flush in case it's needed
	return bs.doFlush(ctx)
}

func (bs *AutobatchBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	// may seem backward to check the backingBs first, but that is the likeliest case
	blk, err := bs.backingBs.Get(ctx, c)
	if err == nil {
		return blk, nil
	}

	if err != ErrNotFound {
		return blk, err
	}

	bs.flushingLk.Lock()
	v, ok := bs.flushingBlksMap[c]
	bs.flushingLk.Unlock()
	if ok {
		return v, nil
	}

	bs.bufferedLk.Lock()
	v, ok = bs.bufferedBlksMap[c]
	bs.bufferedLk.Unlock()
	if ok {
		return v, nil
	}

	return nil, ErrNotFound
}

func (bs *AutobatchBlockstore) DeleteBlock(context.Context, cid.Cid) error {
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
	return nil, xerrors.New("unsupported")

}

func (bs *AutobatchBlockstore) HashOnRead(enabled bool) {
	bs.backingBs.HashOnRead(enabled)
}

func (bs *AutobatchBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	return xerrors.New("unsupported")
}
