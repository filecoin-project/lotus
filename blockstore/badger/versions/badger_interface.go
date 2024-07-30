package versions

import (
	"context"
	"io"

	"github.com/dgraph-io/ristretto"
	"github.com/dgraph-io/ristretto/z"
	"github.com/ipfs/go-cid"
)

// BadgerDB defines the common interface for both v2 and v4 versions of Badger.
type BadgerDB interface {
	Close() error
	IsClosed() bool
	NewStream() BadgerStream
	Update(func(txn Txn) error) error
	View(func(txn Txn) error) error
	NewTransaction(update bool) Txn
	RunValueLogGC(discardRatio float64) error
	Sync() error
	MaxBatchCount() int64
	MaxBatchSize() int64
	IndexCacheMetrics() *ristretto.Metrics
	GetErrKeyNotFound() error
	GetErrNoRewrite() error
	NewWriteBatch() WriteBatch
	Flatten(workers int) error
	Size() (lsm int64, vlog int64)
	Copy(to BadgerDB) error
	DefaultOptions(prefix string, readonly bool) Options
	Load(r io.Reader, maxPendingWrites int) error
	AllKeysChan(ctx context.Context) (<-chan cid.Cid, error)
	DeleteBlock(context.Context, cid.Cid) error
}

// BadgerStream defines the common interface for streaming data in Badger.
type BadgerStream interface {
	SetNumGo(numGo int)
	SetLogPrefix(prefix string)

	Orchestrate(ctx context.Context) error
	ForEach(ctx context.Context, fn func(key string, value string) error) error
}

// Txn defines the common interface for transactions in Badger.
type Txn interface {
	Get(key []byte) (Item, error)
	Set(key, val []byte) error
	Delete(key []byte) error
	Commit() error
	Discard()
	NewIterator(opts IteratorOptions) Iterator
}

type IteratorOptions struct {
	PrefetchSize int
	Prefix       []byte
}

type Iterator interface {
	Next()
	Rewind()
	Seek(key []byte)
	Close()
	Item() Item
	Valid() bool
}

// Item defines the common interface for items in a transaction.
type Item interface {
	Value(fn func([]byte) error) error
	Key() []byte
	Version() uint64
	ValueCopy(dst []byte) ([]byte, error)
	ValueSize() int64
}

type Buffer struct {
	kvList KVList
	buf    z.Buffer
}

type WriteBatch interface {
	Set(key, val []byte) error
	Delete(key []byte) error
	Flush() error
	Cancel()
}

type KVList interface {
	GetKV() []*KV
}

type KV struct {
	Key   []byte
	Value []byte
}
