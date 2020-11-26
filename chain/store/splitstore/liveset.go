package splitstore

import (
	"github.com/bmatsuo/lmdb-go/lmdb"

	cid "github.com/ipfs/go-cid"
)

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
		return txn.Put(s.db, cid.Hash(), markBytes, 0)
	})
}

func (s *liveSet) Has(cid cid.Cid) (has bool, err error) {
	err = s.env.View(func(txn *lmdb.Txn) error {
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
