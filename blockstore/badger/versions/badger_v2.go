package versions

import (
	"context"
	"fmt"
	"io"
	"runtime"

	badger "github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/ristretto"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

// BadgerV2 wraps the Badger v2 database to implement the BadgerDB interface.
type BadgerV2 struct {
	*badger.DB
}

func (b *BadgerV2) Close() error {
	return b.DB.Close()
}

func (b *BadgerV2) IsClosed() bool {
	return b.DB.IsClosed()
}

func (b *BadgerV2) NewStream() BadgerStream {
	return &BadgerV2Stream{b.DB.NewStream()}
}

func (b *BadgerV2) Update(fn func(txn Txn) error) error {
	return b.DB.Update(func(txn *badger.Txn) error {
		return fn(&BadgerV2Txn{txn})
	})
}

func (b *BadgerV2) View(fn func(txn Txn) error) error {
	return b.DB.View(func(txn *badger.Txn) error {
		return fn(&BadgerV2Txn{txn})
	})
}

func (b *BadgerV2) NewTransaction(update bool) Txn {
	return &BadgerV2Txn{b.DB.NewTransaction(update)}
}

func (b *BadgerV2) RunValueLogGC(discardRatio float64) error {
	return b.DB.RunValueLogGC(discardRatio)
}

func (b *BadgerV2) Sync() error {
	return b.DB.Sync()
}

func (b *BadgerV2) MaxBatchCount() int64 {
	return b.DB.MaxBatchCount()
}
func (b *BadgerV2) MaxBatchSize() int64 {
	return b.DB.MaxBatchSize()
}

func (b *BadgerV2) IndexCacheMetrics() *ristretto.Metrics {
	return b.DB.IndexCacheMetrics()
}

func (b *BadgerV2) GetErrKeyNotFound() error {
	return badger.ErrKeyNotFound
}

func (b *BadgerV2) GetErrNoRewrite() error {
	return badger.ErrNoRewrite
}

func (b *BadgerV2) NewWriteBatch() WriteBatch {
	return &BadgerV2WriteBatch{b.DB.NewWriteBatch()}
}

func (b *BadgerV2) Flatten(workers int) error {
	return b.DB.Flatten(workers)
}

func (b *BadgerV2) Size() (lsm int64, vlog int64) {
	return b.DB.Size()
}

func (b *BadgerV2) Copy(ctx context.Context, to BadgerDB) (defErr error) {

	batch := to.NewWriteBatch()
	defer func() {
		if defErr == nil {
			defErr = batch.Flush()
		}
		if defErr != nil {
			batch.Cancel()
		}
	}()

	stream := b.DB.NewStream()

	return iterateBadger(ctx, stream, func(kvs []*pb.KV) error {
		// check whether context is closed on every kv group
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, kv := range kvs {
			if err := batch.Set(kv.Key, kv.Value); err != nil {
				return err
			}
		}
		return nil
	})
}

var IterateLSMWorkers int // defaults to between( 2, 8, runtime.NumCPU/2 )

func iterateBadger(ctx context.Context, stream *badger.Stream, iter func([]*pb.KV) error) error {
	workers := IterateLSMWorkers
	if workers == 0 {
		workers = between(2, 8, runtime.NumCPU()/2)
	}

	stream.NumGo = workers
	stream.LogPrefix = "iterateBadgerKVs"
	stream.Send = func(kvl *pb.KVList) error {
		kvs := make([]*pb.KV, 0, len(kvl.Kv))
		for _, kv := range kvl.Kv {
			if kv.Key != nil && kv.Value != nil {
				kvs = append(kvs, kv)
			}
		}
		if len(kvs) == 0 {
			return nil
		}
		return iter(kvs)
	}
	return stream.Orchestrate(ctx)
}

func between(min, max, val int) int {
	if val > max {
		val = max
	}
	if val < min {
		val = min
	}
	return val
}

func (b *BadgerV2) Load(r io.Reader, maxPendingWrites int) error {
	return b.DB.Load(r, maxPendingWrites)
}

func (b *BadgerV2) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, fmt.Errorf("AllKeysChan is not implemented")
}

func (b *BadgerV2) DeleteBlock(context.Context, cid.Cid) error {
	return fmt.Errorf("DeleteBlock is not implemented")
}

func (b *BadgerV2) Backup(w io.Writer, since uint64) (uint64, error) {
	return b.DB.Backup(w, since)
}

type BadgerV2WriteBatch struct {
	*badger.WriteBatch
}

func (wb *BadgerV2WriteBatch) Set(key, val []byte) error {
	return wb.WriteBatch.Set(key, val)
}

func (wb *BadgerV2WriteBatch) Delete(key []byte) error {
	return wb.WriteBatch.Delete(key)
}

func (wb *BadgerV2WriteBatch) Flush() error {
	return wb.WriteBatch.Flush()
}

func (wb *BadgerV2WriteBatch) Cancel() {
	wb.WriteBatch.Cancel()
}

type BadgerV2Stream struct {
	*badger.Stream
}

func (s *BadgerV2Stream) SetNumGo(numGo int) {
	s.Stream.NumGo = numGo
}

func (s *BadgerV2Stream) SetLogPrefix(prefix string) {
	s.Stream.LogPrefix = prefix
}

func (s *BadgerV2Stream) Orchestrate(ctx context.Context) error {
	return s.Stream.Orchestrate(ctx)
}

func (s *BadgerV2Stream) ForEach(ctx context.Context, fn func(key string, value string) error) error {
	s.Stream.Send = func(list *pb.KVList) error {
		for _, kv := range list.Kv {
			if kv.Key == nil || kv.Value == nil {
				continue
			}
			err := fn(string(kv.Key), string(kv.Value))
			if err != nil {
				return xerrors.Errorf("foreach function: %w", err)
			}

		}
		return nil
	}
	if err := s.Orchestrate(ctx); err != nil {
		return xerrors.Errorf("orchestrate stream: %w", err)
	}
	return nil
}

type BadgerV2Txn struct {
	*badger.Txn
}

func (txn *BadgerV2Txn) Get(key []byte) (Item, error) {
	item, err := txn.Txn.Get(key)
	return &BadgerV2Item{item}, err
}

func (txn *BadgerV2Txn) Set(key, val []byte) error {
	return txn.Txn.Set(key, val)
}

func (txn *BadgerV2Txn) Delete(key []byte) error {
	return txn.Txn.Delete(key)
}

func (txn *BadgerV2Txn) Commit() error {
	return txn.Txn.Commit()
}

func (txn *BadgerV2Txn) Discard() {
	txn.Txn.Discard()
}

func (txn *BadgerV2Txn) NewIterator(opts IteratorOptions) Iterator {
	badgerOpts := badger.DefaultIteratorOptions
	badgerOpts.PrefetchSize = opts.PrefetchSize
	badgerOpts.Prefix = opts.Prefix
	return &BadgerV2Iterator{txn.Txn.NewIterator(badgerOpts)}
}

type BadgerV2Iterator struct {
	*badger.Iterator
}

func (it *BadgerV2Iterator) Next()           { it.Iterator.Next() }
func (it *BadgerV2Iterator) Rewind()         { it.Iterator.Rewind() }
func (it *BadgerV2Iterator) Seek(key []byte) { it.Iterator.Seek(key) }
func (it *BadgerV2Iterator) Close()          { it.Iterator.Close() }
func (it *BadgerV2Iterator) Item() Item      { return &BadgerV2Item{it.Iterator.Item()} }
func (it *BadgerV2Iterator) Valid() bool     { return it.Iterator.Valid() }

type BadgerV2Item struct {
	*badger.Item
}

func (item *BadgerV2Item) Value(fn func([]byte) error) error {
	return item.Item.Value(fn)
}

func (item *BadgerV2Item) Key() []byte {
	return item.Item.Key()
}

func (item *BadgerV2Item) Version() uint64 {
	return item.Item.Version()
}

func (item *BadgerV2Item) ValueCopy(dst []byte) ([]byte, error) {
	return item.Item.ValueCopy(dst)
}

func (item *BadgerV2Item) ValueSize() int64 {
	return item.Item.ValueSize()
}
