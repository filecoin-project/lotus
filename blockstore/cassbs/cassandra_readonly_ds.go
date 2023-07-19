package cassbs

import (
	"context"
	"fmt"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

func ReadonlyError(description string) error {
	return fmt.Errorf("Write protected access attempted on Readonly Blockstore from method %s", description)
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
	return cds.base.Get(ctx, key)
}

func (cds *CassandraDatastoreReadonly) Has(ctx context.Context, key datastore.Key) (bool, error) {
	return cds.base.Has(ctx, key)
}

func (cds *CassandraDatastoreReadonly) GetSize(ctx context.Context, key datastore.Key) (int, error) {
	return cds.base.GetSize(ctx, key)
}

func (cds *CassandraDatastoreReadonly) Query(ctx context.Context, q query.Query) (query.Results, error) {
	return cds.base.Query(ctx, q)
}

func (cds *CassandraDatastoreReadonly) Sync(ctx context.Context, prefix datastore.Key) error {
	return cds.base.Sync(ctx, prefix)
}

func (cds *CassandraDatastoreReadonly) Close() error {
	return cds.base.Close()
}

func (cds *CassandraDatastoreReadonly) Batch(ctx context.Context) (datastore.Batch, error) {
	return nil, ReadonlyError("Batch Create")
}
