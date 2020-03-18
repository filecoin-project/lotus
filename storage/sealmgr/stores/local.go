package stores

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

type StoragePath struct {
	ID     ID
	Weight uint64

	LocalPath string

	CanSeal  bool
	CanStore bool
}

// [path]/sectorstore.json
type StorageMeta struct {
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
	localLk      sync.RWMutex
	localStorage LocalStorage

	paths []*path
}

type path struct {
	lk sync.Mutex

	meta  StorageMeta
	local string

	sectors map[abi.SectorID]sectorbuilder.SectorFileType
}

func NewLocal(ls LocalStorage) (*Local, error) {
	l := &Local{
		localStorage: ls,
	}
	return l, l.open()
}

func (st *Local) OpenPath(p string) error {
	mb, err := ioutil.ReadFile(filepath.Join(p, MetaFile))
	if err != nil {
		return xerrors.Errorf("reading storage metadata for %s: %w", p, err)
	}

	var meta StorageMeta
	if err := json.Unmarshal(mb, &meta); err != nil {
		return xerrors.Errorf("unmarshalling storage metadata for %s: %w", p, err)
	}

	// TODO: Check existing / dedupe

	out := &path{
		meta:    meta,
		local:   p,
		sectors: map[abi.SectorID]sectorbuilder.SectorFileType{},
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

			out.sectors[sid] |= t
		}
	}

	st.paths = append(st.paths, out)

	return nil
}

func (st *Local) open() error {
	st.localLk.Lock()
	defer st.localLk.Unlock()

	cfg, err := st.localStorage.GetStorage()
	if err != nil {
		return xerrors.Errorf("getting local storage config: %w", err)
	}

	for _, path := range cfg.StoragePaths {
		err := st.OpenPath(path.Path)
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

		for _, p := range st.paths {
			p.lk.Lock()
			s, ok := p.sectors[sid]
			p.lk.Unlock()
			if !ok {
				continue
			}
			if s&fileType == 0 {
				continue
			}
			if p.local == "" {
				continue
			}

			spath := filepath.Join(p.local, fileType.String(), sectorutil.SectorName(sid))
			sectorutil.SetPathByType(&out, fileType, spath)
			sectorutil.SetPathByType(&storageIDs, fileType, string(p.meta.ID))

			existing ^= fileType
		}
	}

	for _, fileType := range pathTypes {
		if fileType&allocate == 0 {
			continue
		}

		var best string
		var bestID ID

		for _, p := range st.paths {
			if sealing && !p.meta.CanSeal {
				continue
			}
			if !sealing && !p.meta.CanStore {
				continue
			}

			p.lk.Lock()
			p.sectors[sid] |= fileType
			p.lk.Unlock()

			// TODO: Check free space
			// TODO: Calc weights

			best = filepath.Join(p.local, fileType.String(), sectorutil.SectorName(sid))
			bestID = p.meta.ID
			break // todo: the first path won't always be the best
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

func (st *Local) FindBestAllocStorage(allocate sectorbuilder.SectorFileType, sealing bool) ([]StorageMeta, error) {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	var out []StorageMeta

	for _, p := range st.paths {
		if sealing && !p.meta.CanSeal {
			log.Debugf("alloc: not considering %s; can't seal", p.meta.ID)
			continue
		}
		if !sealing && !p.meta.CanStore {
			log.Debugf("alloc: not considering %s; can't store", p.meta.ID)
			continue
		}

		// TODO: filter out of space

		out = append(out, p.meta)
	}

	if len(out) == 0 {
		return nil, xerrors.New("no good path found")
	}

	// todo: sort by some kind of preference
	return out, nil
}

func (st *Local) FindSector(id abi.SectorID, typ sectorbuilder.SectorFileType) ([]StorageMeta, error) {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	var out []StorageMeta
	for _, p := range st.paths {
		p.lk.Lock()
		t := p.sectors[id]
		if t|typ == 0 {
			continue
		}
		p.lk.Unlock()
		out = append(out, p.meta)
	}
	if len(out) == 0 {
		return nil, xerrors.Errorf("sector %s/s-t0%d-%d not found", typ, id.Miner, id.Number)
	}

	return out, nil
}

func (st *Local) Local() []StoragePath {
	st.localLk.RLock()
	defer st.localLk.RUnlock()

	var out []StoragePath
	for _, p := range st.paths {
		if p.local == "" {
			continue
		}

		out = append(out, StoragePath{
			ID:        p.meta.ID,
			Weight:    p.meta.Weight,
			LocalPath: p.local,
			CanSeal:   p.meta.CanSeal,
			CanStore:  p.meta.CanStore,
		})
	}

	return out
}

func (st *Local) List(id ID) map[abi.SectorID]sectorbuilder.SectorFileType {
	out := map[abi.SectorID]sectorbuilder.SectorFileType{}
	for _, p := range st.paths {
		if p.meta.ID != id { // TODO: not very efficient
			continue
		}

		for id, fileType := range p.sectors {
			out[id] |= fileType
		}
	}
	return out
}
