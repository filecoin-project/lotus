package advmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/node/config"
)

const metaFile = "sectorstore.json"
var pathTypes = []sectorbuilder.SectorFileType{sectorbuilder.FTUnsealed, sectorbuilder.FTSealed, sectorbuilder.FTCache}

type storage struct {
	localLk sync.RWMutex
	localStorage LocalStorage

	paths []path
}

type path struct {
	meta config.StorageMeta
	local string

	sectors map[abi.SectorID]sectorbuilder.SectorFileType
}

func openPath(p string, meta config.StorageMeta) (path, error) {
	out := path{
		meta:    meta,
		local:   p,
		sectors: map[abi.SectorID]sectorbuilder.SectorFileType{},
	}

	for _, t := range pathTypes {
		ents, err := ioutil.ReadDir(filepath.Join(p, t.String()))
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Join(p, t.String()), 0755); err != nil {
					return path{}, xerrors.Errorf("mkdir '%s': %w", filepath.Join(p, t.String()), err)
				}

				continue
			}
			return path{}, xerrors.Errorf("listing %s: %w", filepath.Join(p, t.String()), err)
		}

		for _, ent := range ents {
			sid, err := parseSectorID(ent.Name())
			if err != nil {
				return path{}, xerrors.Errorf("parse sector id %s: %w", ent.Name(), err)
			}

			out.sectors[sid] |= t
		}
	}

	return out, nil
}

func (st *storage) open() error {
	st.localLk.Lock()
	defer st.localLk.Unlock()

	cfg, err := st.localStorage.GetStorage()
	if err != nil {
		return xerrors.Errorf("getting local storage config: %w", err)
	}

	if len(cfg.StoragePaths) == 0 {
		return xerrors.New("no local storage paths configured")
	}

	for _, path := range cfg.StoragePaths {
		mb, err := ioutil.ReadFile(filepath.Join(path.Path, metaFile))
		if err != nil {
			return xerrors.Errorf("reading storage metadata for %s: %w", path.Path, err)
		}

		var meta config.StorageMeta
		if err := json.Unmarshal(mb, &meta); err != nil {
			return xerrors.Errorf("unmarshalling storage metadata for %s: %w", path.Path, err)
		}

		pi, err := openPath(path.Path, meta)
		if err != nil {
			return xerrors.Errorf("opening path %s: %w", path.Path, err)
		}

		st.paths = append(st.paths, pi)
	}

	return nil
}

func (st *storage) acquireSector(mid abi.ActorID, id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	if existing | allocate != existing ^ allocate {
		return sectorbuilder.SectorPaths{}, nil, xerrors.New("can't both find and allocate a sector")
	}

	st.localLk.RLock()

	var out sectorbuilder.SectorPaths

	for _, fileType := range pathTypes {
		if fileType & existing == 0 {
			continue
		}

		for _, p := range st.paths {
			s, ok := p.sectors[abi.SectorID{
				Miner:  mid,
				Number: id,
			}]
			if !ok {
				continue
			}
			if s & fileType == 0 {
				continue
			}

			spath := filepath.Join(p.local, fileType.String(), fmt.Sprintf("s-t0%d-%d", mid, id))

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
		if fileType & allocate == 0 {
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

			s, ok := p.sectors[abi.SectorID{
				Miner:  mid,
				Number: id,
			}]
			if !ok {
				continue
			}
			if s & fileType == 0 {
				continue
			}

			// TODO: Check free space
			// TODO: Calc weights

			best = filepath.Join(p.local, fileType.String(), fmt.Sprintf("s-t0%d-%d", mid, id))
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

func (st *storage) findBestAllocStorage(allocate sectorbuilder.SectorFileType, sealing bool) ([]config.StorageMeta, error) {
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

func (st *storage) findSector(mid abi.ActorID, sn abi.SectorNumber, typ sectorbuilder.SectorFileType) ([]config.StorageMeta, error) {
	var out []config.StorageMeta
	for _, p := range st.paths {
		t := p.sectors[abi.SectorID{
			Miner:  mid,
			Number: sn,
		}]
		if t | typ == 0 {
			continue
		}
		out = append(out, p.meta)
	}
	if len(out) == 0 {
		return nil, xerrors.Errorf("sector %s/s-t0%d-%d not found", typ, mid, sn)
	}

	return out, nil
}

func parseSectorID(baseName string) (abi.SectorID, error) {
	var n abi.SectorNumber
	var mid abi.ActorID
	read, err := fmt.Sscanf(baseName, "s-t0%d-%d", &mid, &n)
	if err != nil {
		return abi.SectorID{}, xerrors.Errorf(": %w", err)
	}

	if read != 2 {
		return abi.SectorID{}, xerrors.Errorf("parseSectorID expected to scan 2 values, got %d", read)
	}

	return abi.SectorID{
		Miner:  mid,
		Number: n,
	}, nil
}