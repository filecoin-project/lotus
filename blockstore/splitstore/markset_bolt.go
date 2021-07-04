package splitstore

import (
	"sync"
	"time"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	bolt "go.etcd.io/bbolt"
)

const boltMarkSetStaging = 16384

type BoltMarkSetEnv struct {
	db *bolt.DB
}

var _ MarkSetEnv = (*BoltMarkSetEnv)(nil)

type BoltMarkSet struct {
	db       *bolt.DB
	bucketId []byte

	// cache for batching
	mx   sync.RWMutex
	pend map[string]struct{}
}

var _ MarkSet = (*BoltMarkSet)(nil)

func NewBoltMarkSetEnv(path string) (*BoltMarkSetEnv, error) {
	db, err := bolt.Open(path, 0644,
		&bolt.Options{
			Timeout: 1 * time.Second,
			NoSync:  true,
		})
	if err != nil {
		return nil, err
	}

	return &BoltMarkSetEnv{db: db}, nil
}

func (e *BoltMarkSetEnv) Create(name string, hint int64) (MarkSet, error) {
	bucketId := []byte(name)
	err := e.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketId)
		if err != nil {
			return xerrors.Errorf("error creating bolt db bucket %s: %w", name, err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &BoltMarkSet{
		db:       e.db,
		bucketId: bucketId,
		pend:     make(map[string]struct{}),
	}, nil
}

func (e *BoltMarkSetEnv) Close() error {
	return e.db.Close()
}

func (s *BoltMarkSet) Mark(cid cid.Cid) error {
	s.mx.Lock()
	defer s.mx.Unlock()

	key := cid.Hash()
	s.pend[string(key)] = struct{}{}

	if len(s.pend) < boltMarkSetStaging {
		return nil
	}

	err := s.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.bucketId)
		for key := range s.pend {
			err := b.Put([]byte(key), markBytes)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	s.pend = make(map[string]struct{})
	return nil
}

func (s *BoltMarkSet) Has(cid cid.Cid) (result bool, err error) {
	s.mx.RLock()
	defer s.mx.RUnlock()

	key := cid.Hash()
	_, result = s.pend[string(key)]
	if result {
		return result, nil
	}

	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.bucketId)
		v := b.Get(key)
		result = v != nil
		return nil
	})

	return result, err
}

func (s *BoltMarkSet) Close() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(s.bucketId)
	})
}
