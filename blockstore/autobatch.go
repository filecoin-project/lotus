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

// contains the same set of blocks twice, once as an ordered list for flushing, and as a map for fast access
type blockBatch struct {
	blockList []block.Block
	blockMap  map[cid.Cid]block.Block
}

type AutobatchBlockstore struct {
	// TODO: drop if memory consumption is too high
	addedCids map[cid.Cid]struct{}

	stateLock     sync.Mutex
	doFlushLock   sync.Mutex
	bufferedBatch blockBatch

	flushingBatch   blockBatch
	flushErr        error
	flushWorkerDone bool

	flushCh chan struct{}

	flushRetryDelay time.Duration
	flushCtx        context.Context
	shutdownCh      chan struct{}

	backingBs Blockstore

	bufferCapacity int
	bufferSize     int
}

func NewAutobatch(ctx context.Context, backingBs Blockstore, bufferCapacity int) *AutobatchBlockstore {
	bs := &AutobatchBlockstore{
		addedCids:      make(map[cid.Cid]struct{}),
		backingBs:      backingBs,
		bufferCapacity: bufferCapacity,
		flushCtx:       ctx,
		flushCh:        make(chan struct{}, 1),
		shutdownCh:     make(chan struct{}),
		// could be made configable
		flushRetryDelay: time.Millisecond * 100,
		flushWorkerDone: false,
	}

	bs.bufferedBatch.blockMap = make(map[cid.Cid]block.Block)

	go bs.flushWorker()

	return bs
}

func (bs *AutobatchBlockstore) Put(ctx context.Context, blk block.Block) error {
	bs.stateLock.Lock()
	defer bs.stateLock.Unlock()

	_, ok := bs.addedCids[blk.Cid()]
	if !ok {
		bs.addedCids[blk.Cid()] = struct{}{}
		bs.bufferedBatch.blockList = append(bs.bufferedBatch.blockList, blk)
		bs.bufferedBatch.blockMap[blk.Cid()] = blk
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

	return nil
}

func (bs *AutobatchBlockstore) flushWorker() {
	defer func() {
		bs.stateLock.Lock()
		bs.flushWorkerDone = true
		bs.stateLock.Unlock()
	}()
	for {
		select {
		case <-bs.flushCh:
			putErr := bs.doFlush(bs.flushCtx)
			for putErr != nil {
				select {
				case <-bs.shutdownCh:
					return
				case <-time.After(bs.flushRetryDelay):
					autolog.Errorf("FLUSH ERRORED: %w, retrying after %v", putErr, bs.flushRetryDelay)
					putErr = bs.doFlush(bs.flushCtx)
				}
			}
		case <-bs.shutdownCh:
			return
		}
	}
}

// caller must NOT hold stateLock
func (bs *AutobatchBlockstore) doFlush(ctx context.Context) error {
	bs.doFlushLock.Lock()
	defer bs.doFlushLock.Unlock()
	if bs.flushErr == nil {
		bs.stateLock.Lock()
		// We do NOT clear addedCids here, because its purpose is to expedite Puts
		bs.flushingBatch = bs.bufferedBatch
		bs.bufferedBatch.blockList = make([]block.Block, 0, len(bs.flushingBatch.blockList))
		bs.bufferedBatch.blockMap = make(map[cid.Cid]block.Block, len(bs.flushingBatch.blockMap))
		bs.stateLock.Unlock()
	}

	bs.flushErr = bs.backingBs.PutMany(ctx, bs.flushingBatch.blockList)
	bs.stateLock.Lock()
	bs.flushingBatch = blockBatch{}
	bs.stateLock.Unlock()

	return bs.flushErr
}

// caller must NOT hold stateLock
func (bs *AutobatchBlockstore) Flush(ctx context.Context) error {
	return bs.doFlush(ctx)
}

func (bs *AutobatchBlockstore) Shutdown(ctx context.Context) error {
	bs.stateLock.Lock()
	flushDone := bs.flushWorkerDone
	bs.stateLock.Unlock()
	if !flushDone {
		// may racily block forever if Shutdown is called in parallel
		bs.shutdownCh <- struct{}{}
	}

	return bs.flushErr
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

	v, ok := bs.flushingBatch.blockMap[c]
	if ok {
		return v, nil
	}

	v, ok = bs.bufferedBatch.blockMap[c]
	if ok {
		return v, nil
	}

	return bs.Get(ctx, c)
}

func (bs *AutobatchBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	// if we wanted to support this, we would have to:
	// - flush
	// - delete from the backingBs (if present)
	// - remove from addedCids (if present)
	// - if present in addedCids, also walk the ordered lists and remove if present
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
	if err := bs.Flush(ctx); err != nil {
		return nil, err
	}

	return bs.backingBs.AllKeysChan(ctx)
}

func (bs *AutobatchBlockstore) HashOnRead(enabled bool) {
	bs.backingBs.HashOnRead(enabled)
}

func (bs *AutobatchBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	blk, err := bs.Get(ctx, cid)
	if err != nil {
		return err
	}

	return callback(blk.RawData())
}
