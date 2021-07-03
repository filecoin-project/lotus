package importmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"

	"github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
)

type Mgr struct {
	mds      *multistore.MultiStore
	ds       datastore.Batching
	repoPath string

	Blockstore blockstore.BasicBlockstore
}

type Label string

const (
	LSource        = "source"    // Function which created the import
	LRootCid       = "root"      // Root CID
	LFileName      = "filename"  // Local file path
	LCARv2FilePath = "CARv2Path" // path of the CARv2 file.
)

func New(mds *multistore.MultiStore, ds datastore.Batching, repoPath string) *Mgr {
	return &Mgr{
		mds:        mds,
		Blockstore: blockstore.Adapt(mds.MultiReadBlockstore()),
		repoPath:   repoPath,

		ds: datastore.NewLogDatastore(namespace.Wrap(ds, datastore.NewKey("/stores")), "storess"),
	}
}

type StoreMeta struct {
	Labels map[string]string
}

func (m *Mgr) NewStore() (multistore.StoreID, *multistore.Store, error) {
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

func (m *Mgr) AddLabel(id multistore.StoreID, key, value string) error { // source, file path, data CID..
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

func (m *Mgr) List() ([]multistore.StoreID, error) {
	var keys []multistore.StoreID

	qres, err := m.ds.Query(query.Query{KeysOnly: true})
	if err != nil {
		return nil, xerrors.Errorf("query error: %w", err)
	}
	defer qres.Close() //nolint:errcheck

	for r := range qres.Next() {
		k := r.Key
		if string(k[0]) == "/" {
			k = k[1:]
		}

		id, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse key %s to uint64, err=%w", r.Key, err)
		}
		keys = append(keys, multistore.StoreID(id))
	}

	return keys, nil
}

func (m *Mgr) Info(id multistore.StoreID) (*StoreMeta, error) {
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

func (m *Mgr) Remove(id multistore.StoreID) error {
	if err := m.mds.Delete(id); err != nil {
		return xerrors.Errorf("removing import: %w", err)
	}

	if err := m.ds.Delete(datastore.NewKey(fmt.Sprintf("%d", id))); err != nil {
		return xerrors.Errorf("removing import metadata: %w", err)
	}

	return nil
}

func (a *Mgr) NewTempFile(id multistore.StoreID) (string, error) {
	file, err := ioutil.TempFile(a.repoPath, fmt.Sprintf("%d", id))
	if err != nil {
		return "", xerrors.Errorf("failed to create temp file: %w", err)
	}

	// close the file as we need to return the path here.
	if err := file.Close(); err != nil {
		return "", xerrors.Errorf("failed to close temp file: %w", err)
	}

	return file.Name(), nil
}
