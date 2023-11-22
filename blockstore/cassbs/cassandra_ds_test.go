package cassbs

import (
	"os"
	"testing"

	dstest "github.com/ipfs/go-datastore/test"
)

func TestCassandraDS(t *testing.T) {
	casConn, ok := os.LookupEnv("CASSANDRA_TEST_CONNECTION")
	if !ok {
		t.Skip("CASSANDRA_TEST_CONNECTION not set")
	}

	ds, err := NewCassandraDS(casConn, "test")
	if err != nil {
		t.Fatal(err)
	}

	dstest.SubtestAll(t, ds)
}
