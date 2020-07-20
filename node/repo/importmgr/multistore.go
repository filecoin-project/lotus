package importmgr

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	"github.com/ipfs/go-datastore/query"
)

type MultiStore struct {
	ds datastore.Batching

	open map[int]*Store
	next int

	lk sync.RWMutex
}

var dsListKey = datastore.NewKey("/list")
var dsMultiKey = datastore.NewKey("/multi")

func NewMultiDstore(ds datastore.Batching) (*MultiStore, error) {
	listBytes, err := ds.Get(dsListKey)
	if xerrors.Is(err, datastore.ErrNotFound) {
		listBytes, _ = json.Marshal([]int{})
	} else if err != nil {
		return nil, xerrors.Errorf("could not read multistore list: %w", err)
	}

	var ids []int
	if err := json.Unmarshal(listBytes, &ids); err != nil {
		return nil, xerrors.Errorf("could not unmarshal multistore list: %w", err)
	}

	mds := &MultiStore{
		ds:   ds,
		open: map[int]*Store{},
	}

	for _, i := range ids {
		if i > mds.next {
			mds.next = i
		}

		_, err := mds.Get(i)
		if err != nil {
			return nil, xerrors.Errorf("open store %d: %w", i, err)
		}
	}

	return mds, nil
}

func (mds *MultiStore) Next() int {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	mds.next++
	return mds.next
}

func (mds *MultiStore) updateStores() error {
	stores := make([]int, 0, len(mds.open))
	for k := range mds.open {
		stores = append(stores, k)
	}
	sort.Ints(stores)

	listBytes, err := json.Marshal(stores)
	if err != nil {
		return xerrors.Errorf("could not marshal list: %w", err)
	}
	err = mds.ds.Put(dsListKey, listBytes)
	if err != nil {
		return xerrors.Errorf("could not save stores list: %w", err)
	}
	return nil
}

func (mds *MultiStore) Get(i int) (*Store, error) {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	store, ok := mds.open[i]
	if ok {
		return store, nil
	}

	wds := ktds.Wrap(mds.ds, ktds.PrefixTransform{
		Prefix: dsMultiKey.ChildString(fmt.Sprintf("%d", i)),
	})

	var err error
	mds.open[i], err = openStore(wds)
	if err != nil {
		return nil, xerrors.Errorf("could not open new store: %w", err)
	}

	err = mds.updateStores()
	if err != nil {
		return nil, xerrors.Errorf("updating stores: %w", err)
	}

	return mds.open[i], nil
}

func (mds *MultiStore) List() []int {
	mds.lk.RLock()
	defer mds.lk.RUnlock()

	out := make([]int, 0, len(mds.open))
	for i := range mds.open {
		out = append(out, i)
	}
	sort.Ints(out)

	return out
}

func (mds *MultiStore) Delete(i int) error {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	store, ok := mds.open[i]
	if !ok {
		return nil
	}
	delete(mds.open, i)
	err := store.Close()
	if err != nil {
		return xerrors.Errorf("closing store: %w", err)
	}

	err = mds.updateStores()
	if err != nil {
		return xerrors.Errorf("updating stores: %w", err)
	}

	qres, err := store.ds.Query(query.Query{KeysOnly: true})
	if err != nil {
		return xerrors.Errorf("query error: %w", err)
	}
	defer qres.Close() //nolint:errcheck

	b, err := store.ds.Batch()
	if err != nil {
		return xerrors.Errorf("batch error: %w", err)
	}

	for r := range qres.Next() {
		if r.Error != nil {
			_ = b.Commit()
			return xerrors.Errorf("iterator error: %w", err)
		}
		err := b.Delete(datastore.NewKey(r.Key))
		if err != nil {
			_ = b.Commit()
			return xerrors.Errorf("adding to batch: %w", err)
		}
	}

	err = b.Commit()
	if err != nil {
		return xerrors.Errorf("committing: %w", err)
	}

	return nil
}

func (mds *MultiStore) Close() error {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	var err error
	for _, s := range mds.open {
		err = multierr.Append(err, s.Close())
	}
	mds.open = make(map[int]*Store)

	return err
}
