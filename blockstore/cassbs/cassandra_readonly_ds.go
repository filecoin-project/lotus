package cassbs

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"

	"github.com/gocql/gocql"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"
)

func ReadonlyError(description string) error {
	return errors.New("Write protected access attempted on Readonly Blockstore from method " + description)
}

type CassandraDatastoreReadonly struct {
	base *CassandraDatastore
}

func NewCassandraDSReadonly(connectString string, nameSpace string) (*CassandraDatastoreReadonly, error) {
	base, err := NewCassandraDS(connectString, nameSpace)
	if err != nil {
		return nil, fmt.Errorf("creating new Cassandra Readonly Datastore: %w", err)
	}
	return &CassandraDatastoreReadonly{base: base}, nil
}

func (cds *CassandraDatastoreReadonly) Put(ctx context.Context, key datastore.Key, value []byte) error {
	return ReadonlyError("Put")
}

func (cds *CassandraDatastoreReadonly) Delete(ctx context.Context, key datastore.Key) error {
	return ReadonlyError("Delete")
}
func (cds *CassandraDatastoreReadonly) DeleteBlock(ctx context.Context, key datastore.Key) error {
	return ReadonlyError("DeleteBlock")
}

func (cds *CassandraDatastoreReadonly) Get(ctx context.Context, key datastore.Key) ([]byte, error) {
	var value []byte
	err := cds.base.session.Query("SELECT value FROM lotus.chain WHERE key = ?", toCasKey(key)).WithContext(ctx).Scan(&value)
	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, datastore.ErrNotFound
		}
		return nil, err
	}
	return value, nil
}

func (cds *CassandraDatastoreReadonly) Has(ctx context.Context, key datastore.Key) (bool, error) {
	var count int
	err := cds.base.session.Query("SELECT COUNT(*) FROM lotus.chain WHERE key = ?", toCasKey(key)).WithContext(ctx).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (cds *CassandraDatastoreReadonly) GetSize(ctx context.Context, key datastore.Key) (int, error) {
	value, err := cds.base.Get(ctx, key) // todo this is not great, but getting blob len is not that easy. Hopefully we don't call this much
	if err != nil {
		return -1, err
	}
	return len(value), nil
}

func (cds *CassandraDatastoreReadonly) Query(ctx context.Context, q query.Query) (query.Results, error) {
	// Basic implementation assumes all filters, orders and limits are applied client-side
	// todo do more fancy things if needed
	iter := cds.base.session.Query("SELECT key, value FROM lotus.chain").WithContext(ctx).Iter()

	var (
		k       string
		v       []byte
		entries []query.Entry
	)

	for iter.Scan(&k, &v) {
		vs := string(v) // copy
		v := []byte(vs)

		k, err := base64.StdEncoding.DecodeString(k)
		if err != nil {
			return nil, xerrors.Errorf("decode key: %w", err)
		}

		entry := query.Entry{Key: string(k), Value: v}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { // todo eww
		return entries[i].Key < entries[j].Key
	})

	if err := iter.Close(); err != nil {
		return nil, err
	}

	// Apply filters, orders and limits client-side
	qr := query.ResultsWithEntries(q, entries)
	for _, filter := range q.Filters {
		qr = query.NaiveFilter(qr, filter)
	}
	qr = query.NaiveOrder(qr, q.Orders...)
	qr = query.NaiveLimit(qr, q.Limit)
	qr = query.NaiveOffset(qr, q.Offset)

	return qr, nil
}

func (cds *CassandraDatastoreReadonly) Sync(ctx context.Context, prefix datastore.Key) error {
	return nil
}

func (cds *CassandraDatastoreReadonly) Close() error {
	cds.base.session.Close()
	return nil
}

func (cds *CassandraDatastoreReadonly) Batch(ctx context.Context) (datastore.Batch, error) {
	return nil, ReadonlyError("Batch Create")
}

func (cds *CassandraDatastore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, nil
}
