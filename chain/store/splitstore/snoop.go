package splitstore

import (
	"context"
	"fmt"
	"os"

	"github.com/bmatsuo/lmdb-go/lmdb"

	cid "github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
)

var TrackingStoreMapSize int64 = 1 << 34 // 16G

type TrackingStore interface {
	Put(cid.Cid, abi.ChainEpoch) error
	PutBatch([]cid.Cid, abi.ChainEpoch) error
	Get(cid.Cid) (abi.ChainEpoch, error)
	Delete(cid.Cid) error
	Keys(context.Context) (<-chan cid.Cid, error)
	Close() error
}

type trackingStore struct {
	env *lmdb.Env
	db  lmdb.DBI
}

func NewTrackingStore(path string) (TrackingStore, error) {
	env, err := lmdb.NewEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LMDB env: %w", err)
	}
	if err = env.SetMapSize(TrackingStoreMapSize); err != nil {
		return nil, fmt.Errorf("failed to set LMDB map size: %w", err)
	}
	if err = env.SetMaxDBs(1); err != nil {
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

	s := new(trackingStore)
	s.env = env
	err = env.Update(func(txn *lmdb.Txn) (err error) {
		s.db, err = txn.CreateDBI("snoop")
		return err
	})

	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *trackingStore) Put(cid cid.Cid, epoch abi.ChainEpoch) error {
	val := epochToBytes(epoch)
	return s.env.Update(func(txn *lmdb.Txn) error {
		return txn.Put(s.db, cid.Hash(), val, 0)
	})
}

func (s *trackingStore) PutBatch(cids []cid.Cid, epoch abi.ChainEpoch) error {
	val := epochToBytes(epoch)
	return s.env.Update(func(txn *lmdb.Txn) error {
		for _, cid := range cids {
			err := txn.Put(s.db, cid.Hash(), val, 0)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *trackingStore) Get(cid cid.Cid) (epoch abi.ChainEpoch, err error) {
	err = s.env.View(func(txn *lmdb.Txn) error {
		txn.RawRead = true

		val, err := txn.Get(s.db, cid.Hash())
		if err != nil {
			return err
		}

		epoch = bytesToEpoch(val)
		return nil
	})

	return
}

func (s *trackingStore) Delete(cid cid.Cid) error {
	return s.env.Update(func(txn *lmdb.Txn) error {
		return txn.Del(s.db, cid.Hash(), nil)
	})
}

func (s *trackingStore) Keys(ctx context.Context) (<-chan cid.Cid, error) {
	ch := make(chan cid.Cid)
	go func() {
		err := s.env.View(func(txn *lmdb.Txn) error {
			defer close(ch)

			txn.RawRead = true
			cur, err := txn.OpenCursor(s.db)
			if err != nil {
				return err
			}
			defer cur.Close()

			for {
				k, _, err := cur.Get(nil, nil, lmdb.Next)
				if err != nil {
					if lmdb.IsNotFound(err) {
						return nil
					}

					return err
				}

				select {
				case ch <- cid.NewCidV1(cid.Raw, k):
				case <-ctx.Done():
					return nil
				}
			}
		})

		if err != nil {
			log.Errorf("error iterating over tracking store keys: %s", err)
		}
	}()

	return ch, nil
}

func (s *trackingStore) Close() error {
	s.env.CloseDBI(s.db)
	return s.env.Close()
}
