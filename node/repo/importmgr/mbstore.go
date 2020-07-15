package importmgr

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

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
		return nil, blockstore.ErrNotFound
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
