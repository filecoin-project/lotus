package imports

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

type ID uint64

func (id ID) dsKey() datastore.Key {
	return datastore.NewKey(fmt.Sprintf("%d", id))
}

type Manager struct {
	ds      datastore.Batching
	rootDir string
	counter *shared.TimeCounter
}

type LabelKey = string
type LabelValue = string

const (
	LSource   = "source"   // Function which created the import
	LRootCid  = "root"     // Root CID
	LFileName = "filename" // Local file path of the source file.
	LCARPath  = "carpath"  // Path of the CARv2 file containing the imported data.
)

func NewManager(ds datastore.Batching, rootDir string) *Manager {
	ds = namespace.Wrap(ds, datastore.NewKey("/stores"))
	ds = datastore.NewLogDatastore(ds, "storess")

	return &Manager{
		ds:      ds,
		rootDir: rootDir,
		counter: shared.NewTimeCounter(),
	}
}

type StoreMeta struct {
	Labels map[LabelKey]LabelValue
}

// CreateImport initializes a new import and returns its ID.
func (m *Manager) CreateImport() (ID, error) {
	id := ID(m.counter.Next())

	meta, err := json.Marshal(&StoreMeta{Labels: map[LabelKey]LabelValue{
		LSource: "unknown",
	}})
	if err != nil {
		return 0, xerrors.Errorf("marshaling empty store metadata: %w", err)
	}

	err = m.ds.Put(id.dsKey(), meta)
	return id, err
}

// AddLabel adds a label associated with an import, such as the source,
// car path, CID, etc.
func (m *Manager) AddLabel(id ID, key LabelKey, value LabelValue) error {
	meta, err := m.ds.Get(id.dsKey())
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

	return m.ds.Put(id.dsKey(), meta)
}

// List returns all import IDs known by this Manager.
func (m *Manager) List() ([]ID, error) {
	var keys []ID

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
		keys = append(keys, ID(id))
	}

	return keys, nil
}

// Info returns the metadata known to this store for the specified import ID.
func (m *Manager) Info(id ID) (*StoreMeta, error) {
	meta, err := m.ds.Get(id.dsKey())
	if err != nil {
		return nil, xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm StoreMeta
	if err := json.Unmarshal(meta, &sm); err != nil {
		return nil, xerrors.Errorf("unmarshaling store meta: %w", err)
	}

	return &sm, nil
}

// Remove drops all data associated with the supplied import ID.
func (m *Manager) Remove(id ID) error {
	if err := m.ds.Delete(id.dsKey()); err != nil {
		return xerrors.Errorf("removing import metadata: %w", err)
	}

	return nil
}

func (m *Manager) FilestoreCARV2FilePathFor(dagRoot cid.Cid) (string, error) {
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
			return info.Labels[LCARPath], nil
		}
	}

	return "", nil
}

func (m *Manager) NewTempFile(importID ID) (string, error) {
	file, err := ioutil.TempFile(m.rootDir, fmt.Sprintf("%d", importID))
	if err != nil {
		return "", xerrors.Errorf("failed to create temp file: %w", err)
	}

	// close the file as we need to return the path here.
	if err := file.Close(); err != nil {
		return "", xerrors.Errorf("failed to close temp file: %w", err)
	}

	return file.Name(), nil
}
