package stores

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

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

	meta  config.StorageMeta
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

	var meta config.StorageMeta
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

func (st *Local) AcquireSector(sid abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	if existing|allocate != existing^allocate {
		return sectorbuilder.SectorPaths{}, nil, xerrors.New("can't both find and allocate a sector")
	}

	st.localLk.RLock()

	var out sectorbuilder.SectorPaths

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

			switch fileType {
			case sectorbuilder.FTUnsealed:
				out.Unsealed = spath
			case sectorbuilder.FTSealed:
				out.Sealed = spath
			case sectorbuilder.FTCache:
				out.Cache = spath
			}

			existing ^= fileType
		}
	}

	for _, fileType := range pathTypes {
		if fileType&allocate == 0 {
			continue
		}

		var best string

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
			break // todo: the first path won't always be the best
		}

		if best == "" {
			st.localLk.RUnlock()
			return sectorbuilder.SectorPaths{}, nil, xerrors.Errorf("couldn't find a suitable path for a sector")
		}

		switch fileType {
		case sectorbuilder.FTUnsealed:
			out.Unsealed = best
		case sectorbuilder.FTSealed:
			out.Sealed = best
		case sectorbuilder.FTCache:
			out.Cache = best
		}

		allocate ^= fileType
	}

	return out, st.localLk.RUnlock, nil
}

func (st *Local) FindBestAllocStorage(allocate sectorbuilder.SectorFileType, sealing bool) ([]config.StorageMeta, error) {
	var out []config.StorageMeta

	for _, p := range st.paths {
		if sealing && !p.meta.CanSeal {
			continue
		}
		if !sealing && !p.meta.CanStore {
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

func (st *Local) FindSector(mid abi.ActorID, sn abi.SectorNumber, typ sectorbuilder.SectorFileType) ([]config.StorageMeta, error) {
	var out []config.StorageMeta
	for _, p := range st.paths {
		p.lk.Lock()
		t := p.sectors[abi.SectorID{
			Miner:  mid,
			Number: sn,
		}]
		if t|typ == 0 {
			continue
		}
		p.lk.Unlock()
		out = append(out, p.meta)
	}
	if len(out) == 0 {
		return nil, xerrors.Errorf("sector %s/s-t0%d-%d not found", typ, mid, sn)
	}

	return out, nil
}

func (st *Local) Local() []api.StoragePath {
	var out []api.StoragePath
	for _, p := range st.paths {
		if p.local == "" {
			continue
		}

		out = append(out, api.StoragePath{
			ID:        p.meta.ID,
			Weight:    p.meta.Weight,
			LocalPath: p.local,
			CanSeal:   p.meta.CanSeal,
			CanStore:  p.meta.CanStore,
		})
	}

	return out
}
