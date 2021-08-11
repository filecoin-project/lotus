package importmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
)

var log = logging.Logger("importmgr")

type ImportID uint64

type Mgr struct {
	ds      datastore.Batching
	tmpDir  string
	counter *shared.TimeCounter
}

type Label string

const (
	LSource                 = "source"    // Function which created the import
	LRootCid                = "root"      // Root CID
	LFileName               = "filename"  // Local file path
	LFileStoreCARv2FilePath = "CARv2Path" // path of the full CARv2 file or a CARv2 file that can serve as the backing store for a Filestore.
)

func New(ds datastore.Batching, tmpDir string) *Mgr {
	return &Mgr{
		tmpDir:  tmpDir,
		ds:      datastore.NewLogDatastore(namespace.Wrap(ds, datastore.NewKey("/stores")), "storess"),
		counter: shared.NewTimeCounter(),
	}
}

type StoreMeta struct {
	Labels map[string]string
}

func (m *Mgr) NewStore() (ImportID, error) {
	id := m.counter.Next()

	meta, err := json.Marshal(&StoreMeta{Labels: map[string]string{
		"source": "unknown",
	}})
	if err != nil {
		return 0, xerrors.Errorf("marshaling empty store metadata: %w", err)
	}

	err = m.ds.Put(datastore.NewKey(fmt.Sprintf("%d", id)), meta)
	return ImportID(id), err
}

func (m *Mgr) AddLabel(id ImportID, key, value string) error { // source, file path, data CID..
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

func (m *Mgr) List() ([]ImportID, error) {
	var keys []ImportID

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
		keys = append(keys, ImportID(id))
	}

	return keys, nil
}

func (m *Mgr) Info(id ImportID) (*StoreMeta, error) {
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

func (m *Mgr) Remove(id ImportID) error {
	if err := m.ds.Delete(datastore.NewKey(fmt.Sprintf("%d", id))); err != nil {
		return xerrors.Errorf("removing import metadata: %w", err)
	}

	return nil
}

func (m *Mgr) FilestoreCARV2FilePathFor(dagRoot cid.Cid) (string, error) {
	importIDs, err := m.List()
	if err != nil {
		return "", xerrors.Errorf("failed to fetch import IDs: %w", err)
	}

	for _, importID := range importIDs {
		info, err := m.Info(importID)
		if err != nil {
			log.Errorf("failed to fetch info, importID=%d: %s", importID, err)
			continue
		}
		if info.Labels[LRootCid] == "" {
			continue
		}
		c, err := cid.Parse(info.Labels[LRootCid])
		if err != nil {
			log.Errorf("failed to parse Root cid %s: %w", info.Labels[LRootCid], err)
			continue
		}
		if c.Equals(dagRoot) {
			return info.Labels[LFileStoreCARv2FilePath], nil
		}
	}

	return "", nil
}

func (m *Mgr) NewTempFile(importID ImportID) (string, error) {
	file, err := ioutil.TempFile(m.tmpDir, fmt.Sprintf("%d", importID))
	if err != nil {
		return "", xerrors.Errorf("failed to create temp file: %w", err)
	}

	// close the file as we need to return the path here.
	if err := file.Close(); err != nil {
		return "", xerrors.Errorf("failed to close temp file: %w", err)
	}

	return file.Name(), nil
}
