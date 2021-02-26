package splitstore

import (
	"os"

	"golang.org/x/xerrors"

	"github.com/ledgerwatch/lmdb-go/lmdb"

	cid "github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
)

var TrackingStoreMapSize int64 = 1 << 34 // 16G; TODO this may be a little too big, we should figure out how to gradually grow the map.

type TrackingStore interface {
	Put(cid.Cid, abi.ChainEpoch) error
	PutBatch([]cid.Cid, abi.ChainEpoch) error
	Get(cid.Cid) (abi.ChainEpoch, error)
	Delete(cid.Cid) error
	ForEach(func(cid.Cid, abi.ChainEpoch) error) error
	Close() error
}

type trackingStore struct {
	env *lmdb.Env
	db  lmdb.DBI
}

func NewTrackingStore(path string) (TrackingStore, error) {
	env, err := lmdb.NewEnv()
	if err != nil {
		return nil, xerrors.Errorf("failed to initialize LMDB env: %w", err)
	}
	if err = env.SetMapSize(TrackingStoreMapSize); err != nil {
		return nil, xerrors.Errorf("failed to set LMDB map size: %w", err)
	}
	if err = env.SetMaxDBs(1); err != nil {
		return nil, xerrors.Errorf("failed to set LMDB max dbs: %w", err)
	}

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

	s := new(trackingStore)
	s.env = env
	err = env.Update(func(txn *lmdb.Txn) (err error) {
		s.db, err = txn.CreateDBI("snoop")
		return err
	})

	if err != nil {
		return nil, xerrors.Errorf("error creating tracking store: %w", err)
	}

	return s, nil
}

func (s *trackingStore) Put(cid cid.Cid, epoch abi.ChainEpoch) error {
	val := epochToBytes(epoch)
	return withMaxReadersRetry(
		func() error {
			return s.env.Update(func(txn *lmdb.Txn) error {
				return txn.Put(s.db, cid.Hash(), val, 0)
			})
		})
}

func (s *trackingStore) PutBatch(cids []cid.Cid, epoch abi.ChainEpoch) error {
	val := epochToBytes(epoch)
	return withMaxReadersRetry(
		func() error {
			return s.env.Update(func(txn *lmdb.Txn) error {
				for _, cid := range cids {
					err := txn.Put(s.db, cid.Hash(), val, 0)
					if err != nil {
						return err
					}
				}

				return nil
			})
		})
}

func (s *trackingStore) Get(cid cid.Cid) (epoch abi.ChainEpoch, err error) {
	err = withMaxReadersRetry(
		func() error {
			return s.env.View(func(txn *lmdb.Txn) error {
				txn.RawRead = true

				val, err := txn.Get(s.db, cid.Hash())
				if err != nil {
					return err
				}

				epoch = bytesToEpoch(val)
				return nil
			})
		})

	return
}

func (s *trackingStore) Delete(cid cid.Cid) error {
	return withMaxReadersRetry(
		func() error {
			return s.env.Update(func(txn *lmdb.Txn) error {
				return txn.Del(s.db, cid.Hash(), nil)
			})
		})
}

func (s *trackingStore) ForEach(f func(cid.Cid, abi.ChainEpoch) error) error {
	return withMaxReadersRetry(
		func() error {
			return s.env.View(func(txn *lmdb.Txn) error {
				txn.RawRead = true
				cur, err := txn.OpenCursor(s.db)
				if err != nil {
					return err
				}
				defer cur.Close()

				for {
					k, v, err := cur.Get(nil, nil, lmdb.Next)
					if err != nil {
						if lmdb.IsNotFound(err) {
							return nil
						}

						return err
					}

					cid := cid.NewCidV1(cid.Raw, k)
					epoch := bytesToEpoch(v)

					err = f(cid, epoch)
					if err != nil {
						return err
					}
				}
			})
		})
}

func (s *trackingStore) Close() error {
	s.env.CloseDBI(s.db)
	return s.env.Close()
}
