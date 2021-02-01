package splitstore

import (
	"math/rand"
	"time"

	"github.com/ledgerwatch/lmdb-go/lmdb"
)

func withMaxReadersRetry(f func() error) error {
retry:
	err := f()
	if lmdb.IsErrno(err, lmdb.ReadersFull) {
		dt := time.Microsecond + time.Duration(rand.Intn(int(time.Millisecond)))
		log.Debugf("MDB_READERS_FULL; retrying operation in %s", dt)
		time.Sleep(dt)
		goto retry
	}

	return err
}
