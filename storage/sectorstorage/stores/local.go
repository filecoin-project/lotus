package stores

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math/bits"
	"os"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sectorstorage/sectorutil"
)

type StoragePath struct {
	ID     ID
	Weight uint64

	LocalPath string

	CanSeal  bool
	CanStore bool
}

// [path]/sectorstore.json
type LocalStorageMeta struct {
	ID     ID
	Weight uint64 // 0 = readonly

	CanSeal  bool
	CanStore bool
}

type LocalStorage interface {
	GetStorage() (config.StorageConfig, error)
	SetStorage(func(*config.StorageConfig)) error
}

const MetaFile = "sectorstore.json"

var pathTypes = []sectorbuilder.SectorFileType{sectorbuilder.FTUnsealed, sectorbuilder.FTSealed, sectorbuilder.FTCache}

type Local struct {
	localStorage LocalStorage
	index        SectorIndex
	urls         []string

	paths map[ID]*path

	localLk sync.RWMutex
}

type path struct {
	local string // absolute local path
}

func NewLocal(ctx context.Context, ls LocalStorage, index SectorIndex, urls []string) (*Local, error) {
	l := &Local{
		localStorage: ls,
		index:        index,
		urls:         urls,

		paths: map[ID]*path{},
	}
	return l, l.open(ctx)
}

func (st *Local) OpenPath(ctx context.Context, p string) error {
	st.localLk.Lock()
	defer st.localLk.Unlock()

	mb, err := ioutil.ReadFile(filepath.Join(p, MetaFile))
	if err != nil {
		return xerrors.Errorf("reading storage metadata for %s: %w", p, err)
	}

	var meta LocalStorageMeta
	if err := json.Unmarshal(mb, &meta); err != nil {
		return xerrors.Errorf("unmarshalling storage metadata for %s: %w", p, err)
	}

	// TODO: Check existing / dedupe

	out := &path{
		local: p,
	}

	fst, err := Stat(p)
	if err != nil {
		return err
	}

	err = st.index.StorageAttach(ctx, StorageInfo{
		ID:       meta.ID,
		URLs:     st.urls,
		Weight:   meta.Weight,
		CanSeal:  meta.CanSeal,
		CanStore: meta.CanStore,
	}, fst)
	if err != nil {
		return xerrors.Errorf("declaring storage in index: %w", err)
	}

	for _, t := range pathTypes {
		ents, err := ioutil.ReadDir(filepath.Join(p, t.String()))
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Join(p, t.String()), 0755); err != nil {
					return xerrors.Errorf("openPath mkdir '%s': %w", filepath.Join(p, t.String()), err)
				}

				continue
			}
			return xerrors.Errorf("listing %s: %w", filepath.Join(p, t.String()), err)
		}

		for _, ent := range ents {
			sid, err := sectorutil.ParseSectorID(ent.Name())
			if err != nil {
				return xerrors.Errorf("parse sector id %s: %w", ent.Name(), err)
			}

			if err := st.index.StorageDeclareSector(ctx, meta.ID, sid, t); err != nil {
				return xerrors.Errorf("declare sector %d(t:%d) -> %s: %w", sid, t, meta.ID, err)
			}
		}
	}

	st.paths[meta.ID] = out

	return nil
}

func (st *Local) open(ctx context.Context) error {
	cfg, err := st.localStorage.GetStorage()
	if err != nil {
		return xerrors.Errorf("getting local storage config: %w", err)
	}

	for _, path := range cfg.StoragePaths {
		err := st.OpenPath(ctx, path.Path)
		if err != nil {
			return xerrors.Errorf("opening path %s: %w", path.Path, err)
		}
	}

	return nil
}

func (st *Local) AcquireSector(ctx context.Context, sid abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, sectorbuilder.SectorPaths, func(), error) {
	if existing|allocate != existing^allocate {
		return sectorbuilder.SectorPaths{}, sectorbuilder.SectorPaths{}, nil, xerrors.New("can't both find and allocate a sector")
	}

	st.localLk.RLock()

	var out sectorbuilder.SectorPaths
	var storageIDs sectorbuilder.SectorPaths

	for _, fileType := range pathTypes {
		if fileType&existing == 0 {
			continue
		}

		si, err := st.index.StorageFindSector(ctx, sid, fileType, false)
		if err != nil {
			log.Warnf("finding existing sector %d(t:%d) failed: %+v", sid, fileType, err)
			continue
		}

		for _, info := range si {
			p, ok := st.paths[info.ID]
			if !ok {
				continue
			}

			if p.local == "" { // TODO: can that even be the case?
				continue
			}

			spath := filepath.Join(p.local, fileType.String(), sectorutil.SectorName(sid))
			sectorutil.SetPathByType(&out, fileType, spath)
			sectorutil.SetPathByType(&storageIDs, fileType, string(info.ID))

			existing ^= fileType
			break
		}
	}

	for _, fileType := range pathTypes {
		if fileType&allocate == 0 {
			continue
		}

		sis, err := st.index.StorageBestAlloc(ctx, fileType, sealing)
		if err != nil {
			st.localLk.RUnlock()
			return sectorbuilder.SectorPaths{}, sectorbuilder.SectorPaths{}, nil, xerrors.Errorf("finding best storage for allocating : %w", err)
		}

		var best string
		var bestID ID

		for _, si := range sis {
			p, ok := st.paths[si.ID]
			if !ok {
				continue
			}

			if p.local == "" { // TODO: can that even be the case?
				continue
			}

			if sealing && !si.CanSeal {
				continue
			}

			if !sealing && !si.CanStore {
				continue
			}

			// TODO: Check free space

			best = filepath.Join(p.local, fileType.String(), sectorutil.SectorName(sid))
			bestID = si.ID
		}

		if best == "" {
			st.localLk.RUnlock()
			return sectorbuilder.SectorPaths{}, sectorbuilder.SectorPaths{}, nil, xerrors.Errorf("couldn't find a suitable path for a sector")
		}

		sectorutil.SetPathByType(&out, fileType, best)
		sectorutil.SetPathByType(&storageIDs, fileType, string(bestID))
		allocate ^= fileType
	}

	return out, storageIDs, st.localLk.RUnlock, nil
}

func (st *Local) Local(ctx context.Context) ([]StoragePath, error) {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	var out []StoragePath
	for id, p := range st.paths {
		if p.local == "" {
			continue
		}

		si, err := st.index.StorageInfo(ctx, id)
		if err != nil {
			return nil, xerrors.Errorf("get storage info for %s: %w", id, err)
		}

		out = append(out, StoragePath{
			ID:        id,
			Weight:    si.Weight,
			LocalPath: p.local,
			CanSeal:   si.CanSeal,
			CanStore:  si.CanStore,
		})
	}

	return out, nil
}

func (st *Local) delete(ctx context.Context, sid abi.SectorID, typ sectorbuilder.SectorFileType) error {
	if bits.OnesCount(uint(typ)) != 1 {
		return xerrors.New("delete expects one file type")
	}

	si, err := st.index.StorageFindSector(ctx, sid, typ, false)
	if err != nil {
		return xerrors.Errorf("finding existing sector %d(t:%d) failed: %w", sid, typ, err)
	}

	for _, info := range si {
		p, ok := st.paths[info.ID]
		if !ok {
			continue
		}

		if p.local == "" { // TODO: can that even be the case?
			continue
		}

		spath := filepath.Join(p.local, typ.String(), sectorutil.SectorName(sid))
		log.Infof("remove %s", spath)

		if err := os.RemoveAll(spath); err != nil {
			log.Errorf("removing sector (%v) from %s: %+v", sid, spath, err)
		}
	}

	return nil
}

func (st *Local) FsStat(id ID) (FsStat, error) {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	p, ok := st.paths[id]
	if !ok {
		return FsStat{}, xerrors.Errorf("fsstat: path not found")
	}

	return Stat(p.local)
}
