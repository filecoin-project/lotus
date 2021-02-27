package splitstore

import (
	"time"

	"golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	bolt "go.etcd.io/bbolt"
)

type BoltLiveSetEnv struct {
	db *bolt.DB
}

var _ LiveSetEnv = (*BoltLiveSetEnv)(nil)

type BoltLiveSet struct {
	db       *bolt.DB
	bucketId []byte
}

var _ LiveSet = (*BoltLiveSet)(nil)

func NewBoltLiveSetEnv(path string) (*BoltLiveSetEnv, error) {
	db, err := bolt.Open(path, 0644,
		&bolt.Options{
			Timeout: 1 * time.Second,
		})
	if err != nil {
		return nil, err
	}

	return &BoltLiveSetEnv{db: db}, nil
}

func (e *BoltLiveSetEnv) NewLiveSet(name string) (LiveSet, error) {
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

	return &BoltLiveSet{db: e.db, bucketId: bucketId}, nil
}

func (e *BoltLiveSetEnv) Close() error {
	return e.db.Close()
}

func (s *BoltLiveSet) Mark(cid cid.Cid) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.bucketId)
		return b.Put(cid.Hash(), markBytes)
	})
}

func (s *BoltLiveSet) Has(cid cid.Cid) (result bool, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.bucketId)
		v := b.Get(cid.Hash())
		result = v != nil
		return nil
	})

	return result, err
}

func (s *BoltLiveSet) Close() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(s.bucketId)
	})
}
