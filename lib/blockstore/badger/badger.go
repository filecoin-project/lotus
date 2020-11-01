package badgerbs

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/dgraph-io/badger/v2"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	logger "github.com/ipfs/go-log/v2"
	pool "github.com/libp2p/go-buffer-pool"

	"github.com/filecoin-project/lotus/lib/blockstore"
)

var (
	ErrBlockstoreClosed = fmt.Errorf("badger blockstore closed")

	log = logger.Logger("badgerbs")
)

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

// badgerLog is a local wrapper for go-log to make the interface
// compatible with badger.Logger (namely, aliasing Warnf to Warningf)
type badgerLog struct {
	logger.ZapEventLogger
}

func (b *badgerLog) Warningf(format string, args ...interface{}) {
	b.Warnf(format, args...)
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
	DB *badger.DB

	// state is guarded by atomic.
	state int64

	prefixing bool
	prefix    []byte
	prefixLen int
}

var _ blockstore.Blockstore = (*Blockstore)(nil)
var _ blockstore.Viewer = (*Blockstore)(nil)
var _ io.Closer = (*Blockstore)(nil)

func Open(opts Options) (*Blockstore, error) {
	opts.Logger = &badgerLog{*log}

	db, err := badger.Open(opts.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger blockstore: %w", err)
	}

	bs := &Blockstore{
		DB: db,
	}

	if p := opts.Prefix; p != "" {
		bs.prefixing = true
		bs.prefix = []byte(p)
		bs.prefixLen = len(bs.prefix)
	}

	return bs, nil
}

func (b *Blockstore) Close() error {
	if !atomic.CompareAndSwapInt64(&b.state, stateOpen, stateClosing) {
		return nil
	}

	defer atomic.StoreInt64(&b.state, stateClosed)
	return b.DB.Close()
}

func (b *Blockstore) View(cid cid.Cid, fn func([]byte) error) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	k, pooled := b.PooledPrefixedKey(cid)
	if pooled {
		defer pool.Put(k)
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

func (b *Blockstore) Has(cid cid.Cid) (bool, error) {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return false, ErrBlockstoreClosed
	}

	k, pooled := b.PooledPrefixedKey(cid)
	if pooled {
		defer pool.Put(k)
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

func (b *Blockstore) Get(cid cid.Cid) (blocks.Block, error) {
	if !cid.Defined() {
		return nil, blockstore.ErrNotFound
	}

	if atomic.LoadInt64(&b.state) != stateOpen {
		return nil, ErrBlockstoreClosed
	}

	k, pooled := b.PooledPrefixedKey(cid)
	if pooled {
		defer pool.Put(k)
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

func (b *Blockstore) GetSize(cid cid.Cid) (int, error) {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return -1, ErrBlockstoreClosed
	}

	k, pooled := b.PooledPrefixedKey(cid)
	if pooled {
		defer pool.Put(k)
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

func (b *Blockstore) Put(block blocks.Block) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	k, pooled := b.PooledPrefixedKey(block.Cid())
	if pooled {
		defer pool.Put(k)
	}

	err := b.DB.Update(func(txn *badger.Txn) error {
		return txn.Set(k, block.RawData())
	})
	if err != nil {
		err = fmt.Errorf("failed to put block in badger blockstore: %w", err)
	}
	return err
}

func (b *Blockstore) PutMany(blocks []blocks.Block) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	batch := b.DB.NewWriteBatch()
	defer batch.Cancel()

	// toReturn tracks the byte slices to return to the pool, if we're using key
	// prefixing. we can't return each slice to the pool after each Set, because
	// badger holds on to the slice.
	var toReturn [][]byte
	if b.prefixing {
		toReturn = make([][]byte, 0, len(blocks))
		defer func() {
			for _, b := range toReturn {
				pool.Put(b)
			}
		}()
	}

	for _, block := range blocks {
		k, pooled := b.PooledPrefixedKey(block.Cid())
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

func (b *Blockstore) DeleteBlock(cid cid.Cid) error {
	if atomic.LoadInt64(&b.state) != stateOpen {
		return ErrBlockstoreClosed
	}

	k, pooled := b.PooledPrefixedKey(cid)
	if pooled {
		defer pool.Put(k)
	}

	return b.DB.Update(func(txn *badger.Txn) error {
		return txn.Delete(k)
	})
}

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
			ch <- cid.NewCidV1(cid.Raw, k)
		}
	}()

	return ch, nil
}

func (b *Blockstore) HashOnRead(enabled bool) {
	log.Warnf("called HashOnRead on badger blockstore; function not supported; ignoring")
}

func (b *Blockstore) PrefixedKey(cid cid.Cid) []byte {
	h := cid.Hash()
	if !b.prefixing {
		return h
	}
	k := make([]byte, b.prefixLen+len(h))
	copy(k, b.prefix)
	copy(k[b.prefixLen:], h)
	return k
}

func (b *Blockstore) PooledPrefixedKey(cid cid.Cid) (key []byte, pooled bool) {
	h := cid.Hash()
	if !b.prefixing {
		return h, false
	}

	size := b.prefixLen + len(h)
	k := pool.Get(size)
	copy(k, b.prefix)
	copy(k[b.prefixLen:], h)
	return k, true
}
