package splitstore

import (
	"fmt"
	"os"

	"github.com/bmatsuo/lmdb-go/lmdb"

	cid "github.com/ipfs/go-cid"
)

var LiveSetMapSize int64 = 1 << 34 // 16G; TODO this may be a little too big, we should figure out how to gradually grow the map.

type LiveSet interface {
	Mark(cid.Cid) error
	Has(cid.Cid) (bool, error)
	Close() error
}

type liveSet struct {
	env *lmdb.Env
	db  lmdb.DBI
}

var markBytes = []byte{}

func NewLiveSetEnv(path string) (*lmdb.Env, error) {
	env, err := lmdb.NewEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LDMB env: %w", err)
	}
	if err = env.SetMapSize(LiveSetMapSize); err != nil {
		return nil, fmt.Errorf("failed to set LMDB map size: %w", err)
	}
	if err = env.SetMaxDBs(2); err != nil {
		return nil, fmt.Errorf("failed to set LMDB max dbs: %w", err)
	}
	if err = env.SetMaxReaders(1); err != nil {
		return nil, fmt.Errorf("failed to set LMDB max readers: %w", err)
	}

	if st, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return nil, fmt.Errorf("failed to create LMDB data directory at %s: %w", path, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat LMDB data dir: %w", err)
	} else if !st.IsDir() {
		return nil, fmt.Errorf("LMDB path is not a directory %s", path)
	}
	err = env.Open(path, lmdb.NoSync|lmdb.WriteMap|lmdb.MapAsync|lmdb.NoReadahead, 0777)
	if err != nil {
		env.Close() //nolint:errcheck
		return nil, fmt.Errorf("error opening LMDB database: %w", err)
	}

	return env, nil
}

func NewLiveSet(env *lmdb.Env, name string) (LiveSet, error) {
	var db lmdb.DBI
	err := env.Update(func(txn *lmdb.Txn) (err error) {
		db, err = txn.CreateDBI(name)
		return
	})

	if err != nil {
		return nil, err
	}

	return &liveSet{env: env, db: db}, nil
}

func (s *liveSet) Mark(cid cid.Cid) error {
	return s.env.Update(func(txn *lmdb.Txn) error {
		err := txn.Put(s.db, cid.Hash(), markBytes, 0)
		if err == nil || lmdb.IsErrno(err, lmdb.KeyExist) {
			return nil
		}
		return err
	})
}

func (s *liveSet) Has(cid cid.Cid) (has bool, err error) {
	err = s.env.View(func(txn *lmdb.Txn) error {
		txn.RawRead = true

		_, err := txn.Get(s.db, cid.Hash())
		if err != nil {
			if lmdb.IsNotFound(err) {
				has = false
				return nil
			}

			return err
		}

		has = true
		return nil
	})

	return
}

func (s *liveSet) Close() error {
	return s.env.Update(func(txn *lmdb.Txn) error {
		return txn.Drop(s.db, true)
	})
}
