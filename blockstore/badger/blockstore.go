package badgerbs

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync/atomic"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/multiformats/go-base32"
	"go.uber.org/zap"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	logger "github.com/ipfs/go-log/v2"
	pool "github.com/libp2p/go-buffer-pool"

	"github.com/filecoin-project/lotus/blockstore"
)

var (
	// KeyPool is the buffer pool we use to compute storage keys.
	KeyPool *pool.BufferPool = pool.GlobalPool
)

var (
	// ErrBlockstoreClosed is returned from blockstore operations after
	// the blockstore has been closed.
	ErrBlockstoreClosed = fmt.Errorf("badger blockstore closed")

	log = logger.Logger("badgerbs")
)

// aliases to mask badger dependencies.
const (
	// FileIO is equivalent to badger/options.FileIO.
	FileIO = options.FileIO
	// MemoryMap is equivalent to badger/options.MemoryMap.
	MemoryMap = options.MemoryMap
	// LoadToRAM is equivalent to badger/options.LoadToRAM.
	LoadToRAM = options.LoadToRAM
)

// Options embeds the badger options themselves, and augments them with
// blockstore-specific options.
type Options struct {
	badger.Options

	// Prefix is an optional prefix to prepend to keys. Default: "".
	Prefix string
}

func DefaultOptions(path string) Options {
	return Options{
		Options: badger.DefaultOptions(path),
		Prefix:  "",
	}
}

// badgerLogger is a local wrapper for go-log to make the interface
// compatible with badger.Logger (namely, aliasing Warnf to Warningf)
type badgerLogger struct {
	*zap.SugaredLogger // skips 1 caller to get useful line info, skipping over badger.Options.

	skip2 *zap.SugaredLogger // skips 2 callers, just like above + this logger.
}

// Warningf is required by the badger logger APIs.
func (b *badgerLogger) Warningf(format string, args ...interface{}) {
	b.skip2.Warnf(format, args...)
}

const (
	stateOpen int64 = iota
	stateClosing
	stateClosed
)

// Blockstore is a badger-backed IPLD blockstore.
//
// NOTE: once Close() is called, methods will try their best to return
// ErrBlockstoreClosed. This will guaranteed to happen for all subsequent
// operation calls after Close() has returned, but it may not happen for
// operations in progress. Those are likely to fail with a different error.
type Blockstore struct {
	// state is accessed atomically
	state int64

	DB *badger.DB

	prefixing bool
	prefix    []byte
	prefixLen int
}

var _ blockstore.Blockstore = (*Blockstore)(nil)
var _ blockstore.Viewer = (*Blockstore)(nil)
var _ io.Closer = (*Blockstore)(nil)

// Open creates a new badger-backed blockstore, with the supplied options.
func Open(opts Options) (*Blockstore, error) {
	opts.Logger = &badgerLogger{
		SugaredLogger: log.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar(),
		skip2:         log.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}

	db, err := badger.Open(opts.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger blockstore: %w", err)
	}

	bs := &Blockstore{DB: db}
	if p := opts.Prefix; p != "" {
		bs.prefixing = true
		bs.prefix = []byte(p)
		bs.prefixLen = len(bs.prefix)
	}

	return bs, nil
}

// Close closes the store. If the store has already been closed, this noops and
// returns an error, even if the first closure resulted in error.
func (b *Blockstore) Close() error {
	if !atomic.CompareAndSwapInt64(&b.state, stateOpen, stateClosing) {
		return nil
	}

	defer atomic.StoreInt64(&b.state, stateClosed)
	return b.DB.Close()
}

// CollectGarbage runs garbage collection on the value log
func (b *Blockstore) CollectGarbage() error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	var err error
	for err == nil {
		err = b.DB.RunValueLogGC(0.125)
	}

	if err == badger.ErrNoRewrite {
		// not really an error in this case
		return nil
	}

	return err
}

// Compact runs a synchronous compaction
func (b *Blockstore) Compact() error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	nworkers := runtime.NumCPU() / 2
	if nworkers < 2 {
		nworkers = 2
	}

	return b.DB.Flatten(nworkers)
}

// View implements blockstore.Viewer, which leverages zero-copy read-only
// access to values.
func (b *Blockstore) View(cid cid.Cid, fn func([]byte) error) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	return b.DB.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(k); err {
		case nil:
			return item.Value(fn)
		case badger.ErrKeyNotFound:
			return blockstore.ErrNotFound
		default:
			return fmt.Errorf("failed to view block from badger blockstore: %w", err)
		}
	})
}

// Has implements Blockstore.Has.
func (b *Blockstore) Has(cid cid.Cid) (bool, error) {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return false, ErrBlockstoreClosed
	}

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	err := b.DB.View(func(txn *badger.Txn) error {
		_, err := txn.Get(k)
		return err
	})

	switch err {
	case badger.ErrKeyNotFound:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, fmt.Errorf("failed to check if block exists in badger blockstore: %w", err)
	}
}

// Get implements Blockstore.Get.
func (b *Blockstore) Get(cid cid.Cid) (blocks.Block, error) {
	if !cid.Defined() {
		return nil, blockstore.ErrNotFound
	}

	if atomic.LoadInt64(&b.state) != stateOpen {
		return nil, ErrBlockstoreClosed
	}

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	var val []byte
	err := b.DB.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(k); err {
		case nil:
			val, err = item.ValueCopy(nil)
			return err
		case badger.ErrKeyNotFound:
			return blockstore.ErrNotFound
		default:
			return fmt.Errorf("failed to get block from badger blockstore: %w", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(val, cid)
}

// GetSize implements Blockstore.GetSize.
func (b *Blockstore) GetSize(cid cid.Cid) (int, error) {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return -1, ErrBlockstoreClosed
	}

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	var size int
	err := b.DB.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(k); err {
		case nil:
			size = int(item.ValueSize())
		case badger.ErrKeyNotFound:
			return blockstore.ErrNotFound
		default:
			return fmt.Errorf("failed to get block size from badger blockstore: %w", err)
		}
		return nil
	})
	if err != nil {
		size = -1
	}
	return size, err
}

// Put implements Blockstore.Put.
func (b *Blockstore) Put(block blocks.Block) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	k, pooled := b.PooledStorageKey(block.Cid())
	if pooled {
		defer KeyPool.Put(k)
	}

	err := b.DB.Update(func(txn *badger.Txn) error {
		return txn.Set(k, block.RawData())
	})
	if err != nil {
		err = fmt.Errorf("failed to put block in badger blockstore: %w", err)
	}
	return err
}

// PutMany implements Blockstore.PutMany.
func (b *Blockstore) PutMany(blocks []blocks.Block) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	// toReturn tracks the byte slices to return to the pool, if we're using key
	// prefixing. we can't return each slice to the pool after each Set, because
	// badger holds on to the slice.
	var toReturn [][]byte
	if b.prefixing {
		toReturn = make([][]byte, 0, len(blocks))
		defer func() {
			for _, b := range toReturn {
				KeyPool.Put(b)
			}
		}()
	}

	batch := b.DB.NewWriteBatch()
	defer batch.Cancel()

	for _, block := range blocks {
		k, pooled := b.PooledStorageKey(block.Cid())
		if pooled {
			toReturn = append(toReturn, k)
		}
		if err := batch.Set(k, block.RawData()); err != nil {
			return err
		}
	}

	err := batch.Flush()
	if err != nil {
		err = fmt.Errorf("failed to put blocks in badger blockstore: %w", err)
	}
	return err
}

// DeleteBlock implements Blockstore.DeleteBlock.
func (b *Blockstore) DeleteBlock(cid cid.Cid) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	k, pooled := b.PooledStorageKey(cid)
	if pooled {
		defer KeyPool.Put(k)
	}

	return b.DB.Update(func(txn *badger.Txn) error {
		return txn.Delete(k)
	})
}

func (b *Blockstore) DeleteMany(cids []cid.Cid) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	// toReturn tracks the byte slices to return to the pool, if we're using key
	// prefixing. we can't return each slice to the pool after each Set, because
	// badger holds on to the slice.
	var toReturn [][]byte
	if b.prefixing {
		toReturn = make([][]byte, 0, len(cids))
		defer func() {
			for _, b := range toReturn {
				KeyPool.Put(b)
			}
		}()
	}

	batch := b.DB.NewWriteBatch()
	defer batch.Cancel()

	for _, cid := range cids {
		k, pooled := b.PooledStorageKey(cid)
		if pooled {
			toReturn = append(toReturn, k)
		}
		if err := batch.Delete(k); err != nil {
			return err
		}
	}

	err := batch.Flush()
	if err != nil {
		err = fmt.Errorf("failed to delete blocks from badger blockstore: %w", err)
	}
	return err
}

// AllKeysChan implements Blockstore.AllKeysChan.
func (b *Blockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return nil, ErrBlockstoreClosed
	}

	txn := b.DB.NewTransaction(false)
	opts := badger.IteratorOptions{PrefetchSize: 100}
	if b.prefixing {
		opts.Prefix = b.prefix
	}
	iter := txn.NewIterator(opts)

	ch := make(chan cid.Cid)
	go func() {
		defer close(ch)
		defer iter.Close()

		// NewCidV1 makes a copy of the multihash buffer, so we can reuse it to
		// contain allocs.
		var buf []byte
		for iter.Rewind(); iter.Valid(); iter.Next() {
			if ctx.Err() != nil {
				return // context has fired.
			}
			if atomic.LoadInt64(&b.state) != stateOpen {
				// open iterators will run even after the database is closed...
				return // closing, yield.
			}
			k := iter.Item().Key()
			if b.prefixing {
				k = k[b.prefixLen:]
			}

			if reqlen := base32.RawStdEncoding.DecodedLen(len(k)); len(buf) < reqlen {
				buf = make([]byte, reqlen)
			}
			if n, err := base32.RawStdEncoding.Decode(buf, k); err == nil {
				select {
				case ch <- cid.NewCidV1(cid.Raw, buf[:n]):
				case <-ctx.Done():
					return
				}
			} else {
				log.Warnf("failed to decode key %s in badger AllKeysChan; err: %s", k, err)
			}
		}
	}()

	return ch, nil
}

// HashOnRead implements Blockstore.HashOnRead. It is not supported by this
// blockstore.
func (b *Blockstore) HashOnRead(_ bool) {
	log.Warnf("called HashOnRead on badger blockstore; function not supported; ignoring")
}

// PooledStorageKey returns the storage key under which this CID is stored.
//
// The key is: prefix + base32_no_padding(cid.Hash)
//
// This method may return pooled byte slice, which MUST be returned to the
// KeyPool if pooled=true, or a leak will occur.
func (b *Blockstore) PooledStorageKey(cid cid.Cid) (key []byte, pooled bool) {
	h := cid.Hash()
	size := base32.RawStdEncoding.EncodedLen(len(h))
	if !b.prefixing { // optimize for branch prediction.
		k := pool.Get(size)
		base32.RawStdEncoding.Encode(k, h)
		return k, true // slicing upto length unnecessary; the pool has already done this.
	}

	size += b.prefixLen
	k := pool.Get(size)
	copy(k, b.prefix)
	base32.RawStdEncoding.Encode(k[b.prefixLen:], h)
	return k, true // slicing upto length unnecessary; the pool has already done this.
}

// Storage acts like PooledStorageKey, but attempts to write the storage key
// into the provided slice. If the slice capacity is insufficient, it allocates
// a new byte slice with enough capacity to accommodate the result. This method
// returns the resulting slice.
func (b *Blockstore) StorageKey(dst []byte, cid cid.Cid) []byte {
	h := cid.Hash()
	reqsize := base32.RawStdEncoding.EncodedLen(len(h)) + b.prefixLen
	if reqsize > cap(dst) {
		// passed slice is smaller than required size; create new.
		dst = make([]byte, reqsize)
	} else if reqsize > len(dst) {
		// passed slice has enough capacity, but its length is
		// restricted, expand.
		dst = dst[:cap(dst)]
	}

	if b.prefixing { // optimize for branch prediction.
		copy(dst, b.prefix)
		base32.RawStdEncoding.Encode(dst[b.prefixLen:], h)
	} else {
		base32.RawStdEncoding.Encode(dst, h)
	}
	return dst[:reqsize]
}
