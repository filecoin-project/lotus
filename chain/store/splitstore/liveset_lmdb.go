package splitstore

import (
	"os"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	"github.com/ledgerwatch/lmdb-go/lmdb"
)

var LMDBLiveSetMapSize int64 = 1 << 34 // 16G; TODO grow the map dynamically

type LMDBLiveSetEnv struct {
	env *lmdb.Env
}

var _ LiveSetEnv = (*LMDBLiveSetEnv)(nil)

type LMDBLiveSet struct {
	env *lmdb.Env
	db  lmdb.DBI
}

var _ LiveSet = (*LMDBLiveSet)(nil)

func NewLMDBLiveSetEnv(path string) (*LMDBLiveSetEnv, error) {
	env, err := lmdb.NewEnv()
	if err != nil {
		return nil, xerrors.Errorf("failed to initialize LDMB env: %w", err)
	}
	if err = env.SetMapSize(LMDBLiveSetMapSize); err != nil {
		return nil, xerrors.Errorf("failed to set LMDB map size: %w", err)
	}
	if err = env.SetMaxDBs(2); err != nil {
		return nil, xerrors.Errorf("failed to set LMDB max dbs: %w", err)
	}
	// if err = env.SetMaxReaders(1); err != nil {
	// 	return nil, xerrors.Errorf("failed to set LMDB max readers: %w", err)
	// }

	if st, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return nil, xerrors.Errorf("failed to create LMDB data directory at %s: %w", path, err)
		}
	} else if err != nil {
		return nil, xerrors.Errorf("failed to stat LMDB data dir: %w", err)
	} else if !st.IsDir() {
		return nil, xerrors.Errorf("LMDB path is not a directory %s", path)
	}
	err = env.Open(path, lmdb.NoSync|lmdb.WriteMap|lmdb.MapAsync|lmdb.NoReadahead, 0777)
	if err != nil {
		env.Close() //nolint:errcheck
		return nil, xerrors.Errorf("error opening LMDB database: %w", err)
	}

	return &LMDBLiveSetEnv{env: env}, nil
}

func (e *LMDBLiveSetEnv) NewLiveSet(name string, hint int64) (LiveSet, error) {
	return NewLMDBLiveSet(e.env, name+".lmdb")
}

func (e *LMDBLiveSetEnv) Close() error {
	return e.env.Close()
}

func NewLMDBLiveSet(env *lmdb.Env, name string) (*LMDBLiveSet, error) {
	var db lmdb.DBI
	err := env.Update(func(txn *lmdb.Txn) (err error) {
		db, err = txn.CreateDBI(name)
		return
	})

	if err != nil {
		return nil, err
	}

	return &LMDBLiveSet{env: env, db: db}, nil
}

func (s *LMDBLiveSet) Mark(cid cid.Cid) error {
	return s.env.Update(func(txn *lmdb.Txn) error {
		err := txn.Put(s.db, cid.Hash(), markBytes, 0)
		if err == nil || lmdb.IsErrno(err, lmdb.KeyExist) {
			return nil
		}
		return err
	})
}

func (s *LMDBLiveSet) Has(cid cid.Cid) (has bool, err error) {
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

func (s *LMDBLiveSet) Close() error {
	return s.env.Update(func(txn *lmdb.Txn) error {
		return txn.Drop(s.db, true)
	})
}
