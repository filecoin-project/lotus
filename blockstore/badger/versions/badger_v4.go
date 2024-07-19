package versions

import (
	"context"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto"
)

// BadgerV4 wraps the Badger v4 database to implement the BadgerDB interface.
type BadgerV4 struct {
	*badger.DB
}

func (b *BadgerV4) Close() error {
	return b.DB.Close()
}

func (b *BadgerV4) IsClosed() bool {
	return b.DB.IsClosed()
}

func (b *BadgerV4) NewStream() BadgerStream {
	return &BadgerV4Stream{b.DB.NewStream()}
}

func (b *BadgerV4) Update(fn func(txn Txn) error) error {
	return b.DB.Update(func(txn *badger.Txn) error {
		return fn(&BadgerV4Txn{txn})
	})
}

func (b *BadgerV4) View(fn func(txn Txn) error) error {
	return b.DB.View(func(txn *badger.Txn) error {
		return fn(&BadgerV4Txn{txn})
	})
}

func (b *BadgerV4) NewTxn(update bool) Txn {
	return &BadgerV4Txn{b.DB.NewTxn(update)}
}

func (b *BadgerV4) RunValueLogGC(discardRatio float64) error {
	return b.DB.RunValueLogGC(discardRatio)
}

func (b *BadgerV4) Sync() error {
	return b.DB.Sync()
}

func (b *BadgerV4) MaxBatchCount() int64 {
	return b.DB.MaxBatchCount()
}

func (b *BadgerV4) MaxBatchSize() int64 {
	return b.DB.MaxBatchSize()
}

func (b *BadgerV4) Subscribe(ctx context.Context, cb func(kv *KVList) error, prefixes ...[]byte) error {
	//todo
	return nil
}

func (b *BadgerV4) BlockCacheMetrics() *ristretto.Metrics {
	return b.DB.BlockCacheMetrics()
}

func (b *BadgerV4) IndexCacheMetrics() *ristretto.Metrics {
	return b.DB.IndexCacheMetrics()
}

type BadgerV4Stream struct {
	*badger.Stream
}

func (s *BadgerV4Stream) SetNumGo(numGo int) {
	s.NumGo = numGo
}

func (s *BadgerV4Stream) SetLogPrefix(prefix string) {
	s.LogPrefix = prefix
}

func (s *BadgerV4Stream) Send(buf *Buffer) error {
	return nil
	/*
		list, err := badger.BufferToKVList(&buf.buf)
		if err != nil {
			return err
		}
		return s.Stream.Send(list)
	*/
}

func (s *BadgerV4Stream) Orchestrate(ctx context.Context) error {
	return s.Stream.Orchestrate(ctx)
}

type BadgerV4Txn struct {
	*badger.Txn
}

func (txn *BadgerV4Txn) Get(key []byte) (Item, error) {
	item, err := txn.Txn.Get(key)
	return &BadgerV4Item{item}, err
}

func (txn *BadgerV4Txn) Set(key, val []byte) error {
	return txn.Txn.Set(key, val)
}

func (txn *BadgerV4Txn) Delete(key []byte) error {
	return txn.Txn.Delete(key)
}

func (txn *BadgerV4Txn) Commit() error {
	return txn.Txn.Commit()
}

func (txn *BadgerV4Txn) Discard() {
	txn.Txn.Discard()
}

type BadgerV4Item struct {
	*badger.Item
}

func (item *BadgerV4Item) Value(fn func([]byte) error) error {
	return item.Item.Value(fn)
}

func (item *BadgerV4Item) Key() []byte {
	return item.Item.Key()
}

func (item *BadgerV4Item) Version() uint64 {
	return item.Item.Version()
}
