package importmgr

import (
	"encoding/json"
	"fmt"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"
)

type Mgr struct {
	mds *MultiStore
	Bs blockstore.Blockstore
	ds datastore.Batching
}

func New(mds *MultiStore, ds datastore.Batching) *Mgr {
	return &Mgr{
		mds: mds,
		bs:  &multiReadBs{
			mds: mds,
		},

		ds: namespace.Wrap(ds, datastore.NewKey("/stores")),
	}
}

type storeMeta struct {
	Labels map[string]string
}

func (m *Mgr) NewStore() (int64, *Store, error) {
	id := m.mds.Next()
	st, err := m.mds.Get(id)
	if err != nil {
		return 0, nil, err
	}

	meta, err := json.Marshal(&storeMeta{Labels: map[string]string{
		"source": "unknown",
	}})
	if err != nil {
		return 0, nil, xerrors.Errorf("marshaling empty store metadata: %w", err)
	}

	err = m.ds.Put(datastore.NewKey(fmt.Sprintf("%d", id)), meta)
	return id, st, err
}

func (m *Mgr) AddLabel(id int64, key, value string) error { // source, file path, data CID..
	meta, err := m.ds.Get(datastore.NewKey(fmt.Sprintf("%d", id)))
	if err != nil {
		return xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm storeMeta
	if err := json.Unmarshal(meta, &sm); err != nil {
		return xerrors.Errorf("unmarshaling store meta: %w", err)
	}

	sm.Labels[key] = value

	meta, err = json.Marshal(&storeMeta{})
	if err != nil {
		return xerrors.Errorf("marshaling store meta: %w", err)
	}

	return m.ds.Put(datastore.NewKey(fmt.Sprintf("%d", id)), meta)
}

// m.List
// m.Info
// m.Delete