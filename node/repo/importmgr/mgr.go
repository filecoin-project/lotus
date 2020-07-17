package importmgr

import (
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

type Mgr struct {
	mds *MultiStore
	ds  datastore.Batching

	Blockstore blockstore.Blockstore
}

type Label string

const (
	LSource   = "source"   // Function which created the import
	LRootCid  = "root"     // Root CID
	LFileName = "filename" // Local file path
	LMTime    = "mtime"    // File modification timestamp
)

func New(mds *MultiStore, ds datastore.Batching) *Mgr {
	return &Mgr{
		mds: mds,
		Blockstore: &multiReadBs{
			mds: mds,
		},

		ds: datastore.NewLogDatastore(namespace.Wrap(ds, datastore.NewKey("/stores")), "storess"),
	}
}

type StoreMeta struct {
	Labels map[string]string
}

func (m *Mgr) NewStore() (int, *Store, error) {
	id := m.mds.Next()
	st, err := m.mds.Get(id)
	if err != nil {
		return 0, nil, err
	}

	meta, err := json.Marshal(&StoreMeta{Labels: map[string]string{
		"source": "unknown",
	}})
	if err != nil {
		return 0, nil, xerrors.Errorf("marshaling empty store metadata: %w", err)
	}

	err = m.ds.Put(datastore.NewKey(fmt.Sprintf("%d", id)), meta)
	return id, st, err
}

func (m *Mgr) AddLabel(id int, key, value string) error { // source, file path, data CID..
	meta, err := m.ds.Get(datastore.NewKey(fmt.Sprintf("%d", id)))
	if err != nil {
		return xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm StoreMeta
	if err := json.Unmarshal(meta, &sm); err != nil {
		return xerrors.Errorf("unmarshaling store meta: %w", err)
	}

	sm.Labels[key] = value

	meta, err = json.Marshal(&sm)
	if err != nil {
		return xerrors.Errorf("marshaling store meta: %w", err)
	}

	return m.ds.Put(datastore.NewKey(fmt.Sprintf("%d", id)), meta)
}

func (m *Mgr) List() []int {
	return m.mds.List()
}

func (m *Mgr) Info(id int) (*StoreMeta, error) {
	meta, err := m.ds.Get(datastore.NewKey(fmt.Sprintf("%d", id)))
	if err != nil {
		return nil, xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm StoreMeta
	if err := json.Unmarshal(meta, &sm); err != nil {
		return nil, xerrors.Errorf("unmarshaling store meta: %w", err)
	}

	return &sm, nil
}

func (m *Mgr) Remove(id int) error {
	if err := m.mds.Delete(id); err != nil {
		return xerrors.Errorf("removing import: %w", err)
	}

	if err := m.ds.Delete(datastore.NewKey(fmt.Sprintf("%d", id))); err != nil {
		return xerrors.Errorf("removing import metadata: %w", err)
	}

	return nil
}
