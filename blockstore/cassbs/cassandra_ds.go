package cassbs

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"os"
	"sort"
	"time"
)

var log = logging.Logger("casbs")

type CassandraDatastore struct {
	session *gocql.Session
}

var ReplicationFactor = 1

func NewCassandraDS(connectString string) (*CassandraDatastore, error) {
	cluster := gocql.NewCluster(connectString)
	cluster.Consistency = gocql.Quorum
	cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: 30}
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("creating new Cassandra session: %w", err)
	}
	if err := setupSchema(session, ReplicationFactor); err != nil {
		return nil, xerrors.Errorf("setup schema: %w", err)
	}
	return &CassandraDatastore{session: session}, nil
}

func setupSchema(session *gocql.Session, replicationFactor int) error {
	if os.Getenv("LOTUS_DROP_CAS") == "1" {
		// drop lotus.chain table
		if err := session.Query(`DROP TABLE IF EXISTS lotus.chain`).WithContext(context.Background()).Exec(); err != nil {
			return fmt.Errorf("dropping table: %w", err)
		}

		// drop keyspace
		if err := session.Query(`DROP KEYSPACE IF EXISTS lotus`).WithContext(context.Background()).Exec(); err != nil {
			return fmt.Errorf("dropping keyspace: %w", err)
		}
	}

	// Set up keyspace if needed
	keyspaceQuery := fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS lotus
		WITH REPLICATION = {
			'class': 'SimpleStrategy',
			'replication_factor': %d
		}
	`, replicationFactor)
	if err := session.Query(keyspaceQuery).WithContext(context.Background()).Exec(); err != nil {
		return fmt.Errorf("creating keyspace: %w", err)
	}

	// Set up table schema if needed
	tableSchemaQuery := `
		CREATE TABLE IF NOT EXISTS lotus.chain (
			key text PRIMARY KEY,
			value blob
		)
	`
	if err := session.Query(tableSchemaQuery).WithContext(context.Background()).Exec(); err != nil {
		return fmt.Errorf("creating table schema: %w", err)
	}

	return nil
}

func toCasKey(key datastore.Key) string {
	return base64.StdEncoding.EncodeToString(key.Bytes())
}

func (cds *CassandraDatastore) Put(ctx context.Context, key datastore.Key, value []byte) error {
	keyStr := toCasKey(key)
	qry := `UPDATE lotus.chain SET value = ? WHERE key = ?`
	if err := cds.session.Query(qry, value, keyStr).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("upserting key-value pair: %w", err)
	}
	return nil
}

func (cds *CassandraDatastore) Delete(ctx context.Context, key datastore.Key) error {
	keyStr := toCasKey(key)
	qry := `DELETE FROM lotus.chain WHERE key = ?`
	if err := cds.session.Query(qry, keyStr).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("deleting key: %w", err)
	}
	return nil
}

var _ datastore.Write = (*CassandraDatastore)(nil)

func (cds *CassandraDatastore) Get(ctx context.Context, key datastore.Key) ([]byte, error) {
	var value []byte
	err := cds.session.Query("SELECT value FROM lotus.chain WHERE key = ?", toCasKey(key)).WithContext(ctx).Scan(&value)
	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, datastore.ErrNotFound
		}
		return nil, err
	}
	return value, nil
}

func (cds *CassandraDatastore) Has(ctx context.Context, key datastore.Key) (bool, error) {
	var count int
	err := cds.session.Query("SELECT COUNT(*) FROM lotus.chain WHERE key = ?", toCasKey(key)).WithContext(ctx).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (cds *CassandraDatastore) GetSize(ctx context.Context, key datastore.Key) (int, error) {
	value, err := cds.Get(ctx, key) // todo this is not great, but getting blob len is not that easy. Hopefully we don't call this much
	if err != nil {
		return -1, err
	}
	return len(value), nil
}

func (cds *CassandraDatastore) Query(ctx context.Context, q query.Query) (query.Results, error) {
	// Basic implementation assumes all filters, orders and limits are applied client-side
	// todo do more fancy things if needed
	iter := cds.session.Query("SELECT key, value FROM lotus.chain").WithContext(ctx).Iter()

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

var _ datastore.Read = (*CassandraDatastore)(nil)

func (cds *CassandraDatastore) Sync(ctx context.Context, prefix datastore.Key) error {
	return nil
}

func (cds *CassandraDatastore) Close() error {
	cds.session.Close()
	return nil
}

var _ datastore.Datastore = (*CassandraDatastore)(nil)

type cassandraBatch struct {
	session *gocql.Session
	batch   *gocql.Batch
}

func (c *cassandraBatch) Put(ctx context.Context, key datastore.Key, value []byte) error {
	statement := "UPDATE lotus.chain SET value = ? WHERE key = ?"
	c.batch.Query(statement, value, toCasKey(key))
	return nil
}

func (c *cassandraBatch) Delete(ctx context.Context, key datastore.Key) error {
	statement := "DELETE FROM lotus.chain WHERE key = ?"
	c.batch.Query(statement, toCasKey(key))
	return nil
}

func (c *cassandraBatch) Commit(ctx context.Context) error {
	var err error
	for i := 0; i < 30; i++ {
		err = c.session.ExecuteBatch(c.batch.WithContext(ctx))
		if err == nil {
			break
		}

		log.Warnf("error executing batch: %s", err)
		time.Sleep(1 * time.Second)
	}

	return err
}

func (cds *CassandraDatastore) Batch(ctx context.Context) (datastore.Batch, error) {
	return &cassandraBatch{
		session: cds.session,
		batch:   cds.session.NewBatch(gocql.UnloggedBatch),
	}, nil
}

var _ datastore.Batching = (*CassandraDatastore)(nil)
