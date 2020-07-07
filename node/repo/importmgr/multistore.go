package importmgr

import (
	"fmt"
	"path"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
)

type dsProvider interface {
	Datastore(namespace string) (datastore.Batching, error)
	ListDatastores(namespace string) ([]int64, error)
	DeleteDatastore(namespace string) error
}

type MultiStore struct {
	provider dsProvider
	namespace string

	open map[int64]*Store
	next int64

	lk sync.RWMutex
}

func NewMultiDstore(provider dsProvider, namespace string) (*MultiStore, error) {
	ids, err := provider.ListDatastores(namespace)
	if err != nil {
		return nil, xerrors.Errorf("listing datastores: %w", err)
	}

	mds := &MultiStore{
		provider:  provider,
		namespace: namespace,
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

func (mds *MultiStore) path(i int64) string {
	return path.Join("/", mds.namespace, fmt.Sprintf("%d", i))
}

func (mds *MultiStore) Next() int64 {
	return atomic.AddInt64(&mds.next, 1)
}

func (mds *MultiStore) Get(i int64) (*Store, error) {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	store, ok := mds.open[i]
	if ok {
		return store, nil
	}

	ds, err := mds.provider.Datastore(mds.path(i))
	if err != nil {
		return nil, err
	}

	mds.open[i], err = openStore(ds)
	return mds.open[i], err
}

func (mds *MultiStore) List() []int64 {
	mds.lk.RLock()
	defer mds.lk.RUnlock()
	out := make([]int64, 0, len(mds.open))
	for i := range mds.open {
		out = append(out, i)
	}

	return out
}

func (mds *MultiStore) Delete(i int64) error {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	store, ok := mds.open[i]
	if ok {
		if err := store.Close(); err != nil {
			return xerrors.Errorf("closing sub-datastore %d: %w", i, err)
		}

		delete(mds.open, i)
	}

	return mds.provider.DeleteDatastore(mds.path(i))
}

func (mds *MultiStore) Close() error {
	mds.lk.Lock()
	defer mds.lk.Unlock()

	var err error
	for i, store := range mds.open {
		cerr := store.Close()
		if cerr != nil {
			err = multierror.Append(err, xerrors.Errorf("closing sub-datastore %d: %w", i, cerr))
		}
	}

	return err
}
