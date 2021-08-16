package imports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"

	"github.com/filecoin-project/go-fil-markets/shared"
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
	CAROwnerImportMgr = "importmgr"
	CAROwnerUser      = "user"
)

const (
	LSource   = LabelKey("source")    // Function which created the import
	LRootCid  = LabelKey("root")      // Root CID
	LFileName = LabelKey("filename")  // Local file path of the source file.
	LCARPath  = LabelKey("car_path")  // Path of the CARv2 file containing the imported data.
	LCAROwner = LabelKey("car_owner") // Owner of the CAR; "importmgr" is us; "user" or empty is them.
)

func NewManager(ds datastore.Batching, rootDir string) *Manager {
	ds = namespace.Wrap(ds, datastore.NewKey("/stores"))
	ds = datastore.NewLogDatastore(ds, "storess")

	m := &Manager{
		ds:      ds,
		rootDir: rootDir,
		counter: shared.NewTimeCounter(),
	}

	log.Info("sanity checking imports")

	ids, err := m.List()
	if err != nil {
		log.Warnw("failed to enumerate imports on initialization", "error", err)
		return m
	}

	var broken int
	for _, id := range ids {
		log := log.With("id", id)

		info, err := m.Info(id)
		if err != nil {
			log.Warnw("failed to query metadata for import; skipping", "error", err)
			continue
		}

		log = log.With("source", info.Labels[LSource], "root", info.Labels[LRootCid], "original", info.Labels[LFileName])

		path, ok := info.Labels[LCARPath]
		if !ok {
			broken++
			log.Warnw("import lacks carv2 path; import will not work; please reimport")
			continue
		}

		stat, err := os.Stat(path)
		if err != nil {
			broken++
			log.Warnw("import has missing/broken carv2; please reimport", "error", err)
			continue
		}

		log.Infow("import ok", "size", stat.Size())
	}

	log.Infow("sanity check completed", "broken", broken, "total", len(ids))

	return m
}

type Meta struct {
	Labels map[LabelKey]LabelValue
}

// CreateImport initializes a new import, returning its ID and optionally a
// CAR path where to place the data, if requested.
func (m *Manager) CreateImport() (id ID, err error) {
	id = ID(m.counter.Next())

	meta := &Meta{Labels: map[LabelKey]LabelValue{
		LSource: "unknown",
	}}

	metajson, err := json.Marshal(meta)
	if err != nil {
		return 0, xerrors.Errorf("marshaling store metadata: %w", err)
	}

	err = m.ds.Put(id.dsKey(), metajson)
	if err != nil {
		return 0, xerrors.Errorf("failed to insert import metadata: %w", err)
	}

	return id, err
}

// AllocateCAR creates a new CAR allocated to the supplied import under the
// root directory.
func (m *Manager) AllocateCAR(id ID) (path string, err error) {
	meta, err := m.ds.Get(id.dsKey())
	if err != nil {
		return "", xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm Meta
	if err := json.Unmarshal(meta, &sm); err != nil {
		return "", xerrors.Errorf("unmarshaling store meta: %w", err)
	}

	// refuse if a CAR path already exists.
	if curr := sm.Labels[LCARPath]; curr != "" {
		return "", xerrors.Errorf("import CAR already exists at %s: %w", curr, err)
	}

	path = filepath.Join(m.rootDir, fmt.Sprintf("%d.car", id))
	file, err := os.Create(path)
	if err != nil {
		return "", xerrors.Errorf("failed to create car file for import: %w", err)
	}

	// close the file before returning the path.
	if err := file.Close(); err != nil {
		return "", xerrors.Errorf("failed to close temp file: %w", err)
	}

	// record the path and ownership.
	sm.Labels[LCARPath] = path
	sm.Labels[LCAROwner] = CAROwnerImportMgr

	if meta, err = json.Marshal(sm); err != nil {
		return "", xerrors.Errorf("marshaling store metadata: %w", err)
	}

	err = m.ds.Put(id.dsKey(), meta)
	return path, err
}

// AddLabel adds a label associated with an import, such as the source,
// car path, CID, etc.
func (m *Manager) AddLabel(id ID, key LabelKey, value LabelValue) error {
	meta, err := m.ds.Get(id.dsKey())
	if err != nil {
		return xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm Meta
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
func (m *Manager) Info(id ID) (*Meta, error) {
	meta, err := m.ds.Get(id.dsKey())
	if err != nil {
		return nil, xerrors.Errorf("getting metadata form datastore: %w", err)
	}

	var sm Meta
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

func (m *Manager) CARPathFor(dagRoot cid.Cid) (string, error) {
	ids, err := m.List()
	if err != nil {
		return "", xerrors.Errorf("failed to fetch import IDs: %w", err)
	}

	for _, id := range ids {
		info, err := m.Info(id)
		if err != nil {
			log.Errorf("failed to fetch info, importID=%d: %s", id, err)
			continue
		}
		if info.Labels[LRootCid] == "" {
			continue
		}
		c, err := cid.Parse(info.Labels[LRootCid])
		if err != nil {
			log.Errorf("failed to parse root cid %s: %s", info.Labels[LRootCid], err)
			continue
		}
		if c.Equals(dagRoot) {
			return info.Labels[LCARPath], nil
		}
	}

	return "", nil
}
