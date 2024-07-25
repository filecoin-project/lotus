package versions

import (
	"context"

	"github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/ristretto"
	"github.com/dgraph-io/ristretto/z"
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
	Subscribe(ctx context.Context, cb func(kv *KVList) error, prefixes ...[]byte) error
	BlockCacheMetrics() *ristretto.Metrics
	IndexCacheMetrics() *ristretto.Metrics
	GetErrKeyNotFound() error
	GetErrNoRewrite() error
	NewWriteBatch() WriteBatch
	Flatten(workers int) error
	Size()  (lsm int64, vlog int64)

}

// BadgerStream defines the common interface for streaming data in Badger.
type BadgerStream interface {
	SetNumGo(numGo int)
	SetLogPrefix(prefix string)
	Send(buf *Buffer) error
	Orchestrate(ctx context.Context) error
}

// Txn defines the common interface for transactions in Badger.
type Txn interface {
	Get(key []byte) (Item, error)
	Set(key, val []byte) error
	Delete(key []byte) error
	Commit() error
	Discard()
}

// Item defines the common interface for items in a transaction.
type Item interface {
	Value(fn func([]byte) error) error
	Key() []byte
	Version() uint64
	ValueCopy(dst []byte) ([]byte, error)
	ValueSize() int64
}

// KVList is an alias for the KVList type from the Badger package.
type KVList = pb.KVList

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
