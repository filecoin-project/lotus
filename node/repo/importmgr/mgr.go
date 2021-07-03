package importmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
)

type Mgr struct {
	ds       datastore.Batching
	repoPath string
	counter  *shared.TimeCounter
}

type Label string

const (
	LSource        = "source"    // Function which created the import
	LRootCid       = "root"      // Root CID
	LFileName      = "filename"  // Local file path
	LCARv2FilePath = "CARv2Path" // path of the CARv2 file.
)

func New(ds datastore.Batching, repoPath string) *Mgr {
	return &Mgr{
		repoPath: repoPath,
		ds:       datastore.NewLogDatastore(namespace.Wrap(ds, datastore.NewKey("/stores")), "storess"),
		counter:  shared.NewTimeCounter(),
	}
}

type StoreMeta struct {
	Labels map[string]string
}

func (m *Mgr) NewStore() (uint64, error) {
	id := m.counter.Next()

	meta, err := json.Marshal(&StoreMeta{Labels: map[string]string{
		"source": "unknown",
	}})
	if err != nil {
		return 0, xerrors.Errorf("marshaling empty store metadata: %w", err)
	}

	err = m.ds.Put(datastore.NewKey(fmt.Sprintf("%d", id)), meta)
	return id, err
}

func (m *Mgr) AddLabel(id uint64, key, value string) error { // source, file path, data CID..
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

func (m *Mgr) List() ([]uint64, error) {
	var keys []uint64

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
		keys = append(keys, id)
	}

	return keys, nil
}

func (m *Mgr) Info(id uint64) (*StoreMeta, error) {
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

func (m *Mgr) Remove(id uint64) error {
	if err := m.ds.Delete(datastore.NewKey(fmt.Sprintf("%d", id))); err != nil {
		return xerrors.Errorf("removing import metadata: %w", err)
	}

	return nil
}

func (m *Mgr) NewTempFile(id uint64) (string, error) {
	file, err := ioutil.TempFile(m.repoPath, fmt.Sprintf("%d", id))
	if err != nil {
		return "", xerrors.Errorf("failed to create temp file: %w", err)
	}

	// close the file as we need to return the path here.
	if err := file.Close(); err != nil {
		return "", xerrors.Errorf("failed to close temp file: %w", err)
	}

	return file.Name(), nil
}
