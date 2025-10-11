package blockstore

import (
	"context"
	"sync"
	"time"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"golang.org/x/xerrors"
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
	bufferedBatch blockBatch

	flushingBatch blockBatch
	flushErr      error

	flushCh chan struct{}

	doFlushLock     sync.Mutex
	flushRetryDelay time.Duration
	doneCh          chan struct{}
	shutdown        context.CancelFunc

	backingBs Blockstore

	bufferCapacity int
	bufferSize     int
}

func NewAutobatch(ctx context.Context, backingBs Blockstore, bufferCapacity int) *AutobatchBlockstore {
	ctx, cancel := context.WithCancel(ctx)
	bs := &AutobatchBlockstore{
		addedCids:      make(map[cid.Cid]struct{}),
		backingBs:      backingBs,
		bufferCapacity: bufferCapacity,
		flushCh:        make(chan struct{}, 1),
		doneCh:         make(chan struct{}),
		// could be made configurable
		flushRetryDelay: time.Millisecond * 100,
		shutdown:        cancel,
	}

	bs.bufferedBatch.blockMap = make(map[cid.Cid]block.Block)

	go bs.flushWorker(ctx)

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

func (bs *AutobatchBlockstore) flushWorker(ctx context.Context) {
	defer close(bs.doneCh)
	for {
		select {
		case <-bs.flushCh:
			// TODO: check if we _should_ actually flush. We could get a spurious wakeup
			// here.
			putErr := bs.doFlush(ctx, false)
			for putErr != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(bs.flushRetryDelay):
					autolog.Errorf("FLUSH ERRORED: %w, retrying after %v", putErr, bs.flushRetryDelay)
					putErr = bs.doFlush(ctx, true)
				}
			}
		case <-ctx.Done():
			// Do one last flush.
			_ = bs.doFlush(ctx, false)
			return
		}
	}
}

// caller must NOT hold stateLock
// set retryOnly to true to only retry a failed flush and not flush anything new.
func (bs *AutobatchBlockstore) doFlush(ctx context.Context, retryOnly bool) error {
	bs.doFlushLock.Lock()
	defer bs.doFlushLock.Unlock()

	// If we failed to flush last time, try flushing again.
	if bs.flushErr != nil {
		bs.flushErr = bs.backingBs.PutMany(ctx, bs.flushingBatch.blockList)
	}

	// If we failed, or we're _only_ retrying, bail.
	if retryOnly || bs.flushErr != nil {
		return bs.flushErr
	}

	// Then take the current batch...
	bs.stateLock.Lock()
	// We do NOT clear addedCids here, because its purpose is to expedite Puts
	bs.flushingBatch = bs.bufferedBatch
	bs.bufferedBatch.blockList = make([]block.Block, 0, len(bs.flushingBatch.blockList))
	bs.bufferedBatch.blockMap = make(map[cid.Cid]block.Block, len(bs.flushingBatch.blockMap))
	bs.stateLock.Unlock()

	// And try to flush it.
	bs.flushErr = bs.backingBs.PutMany(ctx, bs.flushingBatch.blockList)

	// If we succeeded, reset the batch. Otherwise, we'll try again next time.
	if bs.flushErr == nil {
		bs.stateLock.Lock()
		bs.flushingBatch = blockBatch{}
		bs.stateLock.Unlock()
	}

	return bs.flushErr
}

// Flush caller must NOT hold stateLock
func (bs *AutobatchBlockstore) Flush(ctx context.Context) error {
	return bs.doFlush(ctx, false)
}

func (bs *AutobatchBlockstore) Shutdown(ctx context.Context) error {
	// TODO: Prevent puts after we call this to avoid losing data.
	bs.shutdown()
	select {
	case <-bs.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}

	bs.doFlushLock.Lock()
	defer bs.doFlushLock.Unlock()

	return bs.flushErr
}

func (bs *AutobatchBlockstore) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	// may seem backward to check the backingBs first, but that is the likeliest case
	blk, err := bs.backingBs.Get(ctx, c)
	if err == nil {
		return blk, nil
	}

	if !ipld.IsNotFound(err) {
		return blk, err
	}

	bs.stateLock.Lock()
	v, ok := bs.flushingBatch.blockMap[c]
	if ok {
		bs.stateLock.Unlock()
		return v, nil
	}

	v, ok = bs.bufferedBatch.blockMap[c]
	if ok {
		bs.stateLock.Unlock()
		return v, nil
	}
	bs.stateLock.Unlock()

	// We have to check the backing store one more time because it may have been flushed by the
	// time we were able to take the lock above.
	return bs.backingBs.Get(ctx, c)
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
	if ipld.IsNotFound(err) {
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

func (bs *AutobatchBlockstore) View(ctx context.Context, cid cid.Cid, callback func([]byte) error) error {
	blk, err := bs.Get(ctx, cid)
	if err != nil {
		return err
	}

	return callback(blk.RawData())
}
