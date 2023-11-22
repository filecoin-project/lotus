package cassbs

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
)

var log = logging.Logger("casbs")

type CassandraDatastore struct {
	session   *gocql.Session
	nameSpace string
}

var ReplicationFactor = 2

func NewCassandraDS(connectString string, nameSpace string) (*CassandraDatastore, error) {
	cluster := gocql.NewCluster(connectString)
	cluster.Consistency = gocql.Quorum
	cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: 30}
	cluster.Timeout = 30 * time.Second
	cluster.ConnectTimeout = 30 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("creating new Cassandra session: %w", err)
	}
	if err := setupSchema(session, ReplicationFactor, nameSpace); err != nil {
		return nil, xerrors.Errorf("setup schema: %w", err)
	}
	return &CassandraDatastore{session: session, nameSpace: nameSpace}, nil
}

func setupSchema(session *gocql.Session, replicationFactor int, nameSpace string) error {
	if os.Getenv("LOTUS_DROP_CAS") == "1" {
		// drop table
		if err := session.Query(fmt.Sprintf(`DROP TABLE IF EXISTS %s.%s`, nameSpace, nameSpace)).WithContext(context.Background()).Exec(); err != nil {
			return fmt.Errorf("dropping table: %w", err)
		}

		// drop keyspace
		if err := session.Query(fmt.Sprintf(`DROP KEYSPACE IF EXISTS %s`, nameSpace)).WithContext(context.Background()).Exec(); err != nil {
			return fmt.Errorf("dropping keyspace: %w", err)
		}
	}

	// Set up keyspace if needed
	keyspaceQuery := fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS %s
		WITH REPLICATION = {
			'class': 'SimpleStrategy',
			'replication_factor': %d
		}
	`, nameSpace, replicationFactor)
	if err := session.Query(keyspaceQuery).WithContext(context.Background()).Exec(); err != nil {
		return fmt.Errorf("creating keyspace: %w", err)
	}

	// Set up table schema if needed
	tableSchemaQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			key text PRIMARY KEY,
			value blob
		)
	`, nameSpace, nameSpace)
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
	qry := fmt.Sprintf(`UPDATE %s.%s SET value = ? WHERE key = ?`, cds.nameSpace, cds.nameSpace)
	if err := cds.session.Query(qry, value, keyStr).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("upserting key-value pair: %w", err)
	}
	return nil
}

func (cds *CassandraDatastore) Delete(ctx context.Context, key datastore.Key) error {
	keyStr := toCasKey(key)
	qry := fmt.Sprintf(`DELETE FROM %s.%s WHERE key = ?`, cds.nameSpace, cds.nameSpace)
	if err := cds.session.Query(qry, keyStr).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("deleting key: %w", err)
	}
	return nil
}

var _ datastore.Write = (*CassandraDatastore)(nil)

func (cds *CassandraDatastore) Get(ctx context.Context, key datastore.Key) ([]byte, error) {
	var value []byte
	err := cds.session.Query(fmt.Sprintf("SELECT value FROM %s.%s WHERE key = ?", cds.nameSpace, cds.nameSpace), toCasKey(key)).WithContext(ctx).Scan(&value)
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
	err := cds.session.Query(fmt.Sprintf("SELECT COUNT(*) FROM %s.%s WHERE key = ?", cds.nameSpace, cds.nameSpace), toCasKey(key)).WithContext(ctx).Scan(&count)
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
	// Basic implementation assumes all filters, orders, and limits are applied client-side
	// todo do more fancy things if needed
	iter := cds.session.Query(fmt.Sprintf("SELECT key, value FROM %s.%s", cds.nameSpace, cds.nameSpace)).WithContext(ctx).Iter()

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

	// Apply filters, orders, and limits client-side
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
	session   *gocql.Session
	batch     *gocql.Batch
	wg        sync.WaitGroup
	nameSpace string
}

func (c *cassandraBatch) Put(ctx context.Context, key datastore.Key, value []byte) error {
	statement := fmt.Sprintf("UPDATE %s.%s SET value = ? WHERE key = ?", c.nameSpace, c.nameSpace)
	c.batch.Query(statement, value, toCasKey(key))

	// todo setting a batch size of 1 violates the whole point of the batch writes
	// however with batch writes, there is some sort of race condition where the leader
	// will update the chain head and notify the followers before the data has all been commited to the db
	// this means that the followers will be behind because they do not see the latest chain head when the
	// hello service notifies them of a new block
	if c.batch.Size() >= 1 {
		c.wg.Add(1)
		go func(batch *gocql.Batch) {
			defer c.wg.Done()
			err := c.session.ExecuteBatch(batch.WithContext(ctx))
			if err != nil {
				log.Warnf("error executing batch: %s", err)
			}
		}(c.batch)
		c.batch = c.session.NewBatch(gocql.UnloggedBatch)
	}
	return nil
}

func (c *cassandraBatch) Delete(ctx context.Context, key datastore.Key) error {
	statement := fmt.Sprintf("DELETE FROM %s.%s WHERE key = ?", c.nameSpace, c.nameSpace)
	c.batch.Query(statement, toCasKey(key))
	return nil
}

func (c *cassandraBatch) Commit(ctx context.Context) error {
	c.wg.Wait()

	if c.batch.Size() == 0 {
		return nil
	}

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
		session:   cds.session,
		batch:     cds.session.NewBatch(gocql.UnloggedBatch),
		nameSpace: cds.nameSpace,
	}, nil
}

var _ datastore.Batching = (*CassandraDatastore)(nil)
