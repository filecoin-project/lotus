package splitstore

import (
	"math/rand"
	"time"

	"golang.org/x/xerrors"

	"github.com/ledgerwatch/lmdb-go/lmdb"
)

func withMaxReadersRetry(f func() error) error {
retry:
	err := f()
	if err != nil && lmdb.IsErrno(err, lmdb.ReadersFull) {
		dt := time.Microsecond + time.Duration(rand.Intn(int(10*time.Microsecond)))
		log.Debugf("MDB_READERS_FULL; retrying operation in %s", dt)
		time.Sleep(dt)
		goto retry
	}

	if err != nil {
		return xerrors.Errorf("error performing lmdb operation: %w", err)
	}

	return nil
}
