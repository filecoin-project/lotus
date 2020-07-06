package importmgr

import (
	"context"
	"fmt"
	"path"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/go-multierror"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
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

type multiReadBs struct {
	// TODO: some caching
	mds *MultiStore
}

func (m *multiReadBs) Has(cid cid.Cid) (bool, error) {
	m.mds.lk.RLock()
	defer m.mds.lk.RUnlock()

	var merr error
	for i, store := range m.mds.open {
		has, err := store.Bstore.Has(cid)
		if err != nil {
			merr = multierror.Append(merr, xerrors.Errorf("has (ds %d): %w", i, err))
			continue
		}
		if !has {
			continue
		}

		return true, nil
	}

	return false, merr
}

func (m *multiReadBs) Get(cid cid.Cid) (blocks.Block, error) {
	m.mds.lk.RLock()
	defer m.mds.lk.RUnlock()

	var merr error
	for i, store := range m.mds.open {
		has, err := store.Bstore.Has(cid)
		if err != nil {
			merr = multierror.Append(merr, xerrors.Errorf("has (ds %d): %w", i, err))
			continue
		}
		if !has {
			continue
		}

		val, err := store.Bstore.Get(cid)
		if err != nil {
			merr = multierror.Append(merr, xerrors.Errorf("get (ds %d): %w", i, err))
			continue
		}

		return val, nil
	}

	if merr == nil {
		return nil, datastore.ErrNotFound
	}

	return nil, merr
}

func (m *multiReadBs) DeleteBlock(cid cid.Cid) error {
	return xerrors.Errorf("operation not supported")
}

func (m *multiReadBs) GetSize(cid cid.Cid) (int, error) {
	return 0, xerrors.Errorf("operation not supported")
}

func (m *multiReadBs) Put(block blocks.Block) error {
	return xerrors.Errorf("operation not supported")
}

func (m *multiReadBs) PutMany(blocks []blocks.Block) error {
	return xerrors.Errorf("operation not supported")
}

func (m *multiReadBs) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, xerrors.Errorf("operation not supported")
}

func (m *multiReadBs) HashOnRead(enabled bool) {
	return
}

var _ blockstore.Blockstore = &multiReadBs{}