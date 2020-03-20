package stores

import (
	"context"
	"net/url"
	gopath "path"
	"sort"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

// ID identifies sector storage by UUID. One sector storage should map to one
//  filesystem, local or networked / shared by multiple machines
type ID string

type StorageInfo struct {
	ID     ID
	URLs   []string // TODO: Support non-http transports
	Weight uint64

	CanSeal  bool
	CanStore bool
}

type SectorIndex interface { // part of storage-miner api
	StorageAttach(context.Context, StorageInfo, FsStat) error
	StorageInfo(context.Context, ID) (StorageInfo, error)
	// TODO: StorageUpdateStats(FsStat)

	StorageDeclareSector(ctx context.Context, storageId ID, s abi.SectorID, ft sectorbuilder.SectorFileType) error
	StorageFindSector(context.Context, abi.SectorID, sectorbuilder.SectorFileType) ([]StorageInfo, error)

	StorageBestAlloc(ctx context.Context, allocate sectorbuilder.SectorFileType, sealing bool) ([]StorageInfo, error)
}

type Decl struct {
	abi.SectorID
	sectorbuilder.SectorFileType
}

type storageEntry struct {
	info *StorageInfo
	fsi  FsStat
}

type Index struct {
	lk sync.RWMutex

	sectors map[Decl][]ID
	stores  map[ID]*storageEntry
}

func NewIndex() *Index {
	return &Index{
		sectors: map[Decl][]ID{},
		stores:  map[ID]*storageEntry{},
	}
}

func (i *Index) StorageList(ctx context.Context) (map[ID][]Decl, error) {
	byID := map[ID]map[abi.SectorID]sectorbuilder.SectorFileType{}

	for id := range i.stores {
		byID[id] = map[abi.SectorID]sectorbuilder.SectorFileType{}
	}
	for decl, ids := range i.sectors {
		for _, id := range ids {
			byID[id][decl.SectorID] |= decl.SectorFileType
		}
	}

	out := map[ID][]Decl{}
	for id, m := range byID {
		out[id] = []Decl{}
		for sectorID, fileType := range m {
			out[id] = append(out[id], Decl{
				SectorID:       sectorID,
				SectorFileType: fileType,
			})
		}
	}

	return out, nil
}

func (i *Index) StorageAttach(ctx context.Context, si StorageInfo, st FsStat) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	log.Infof("New sector storage: %s", si.ID)

	if _, ok := i.stores[si.ID]; ok {
		for _, u := range si.URLs {
			if _, err := url.Parse(u); err != nil {
				return xerrors.Errorf("failed to parse url %s: %w", si.URLs, err)
			}
		}

		i.stores[si.ID].info.URLs = append(i.stores[si.ID].info.URLs, si.URLs...)
		return nil
	}
	i.stores[si.ID] = &storageEntry{
		info: &si,
		fsi:  st,
	}
	return nil
}

func (i *Index) StorageDeclareSector(ctx context.Context, storageId ID, s abi.SectorID, ft sectorbuilder.SectorFileType) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	for _, fileType := range pathTypes {
		if fileType&ft == 0 {
			continue
		}

		d := Decl{s, fileType}

		for _, sid := range i.sectors[d] {
			if sid == storageId {
				log.Warnf("sector %v redeclared in %s", storageId)
				return nil
			}
		}

		i.sectors[d] = append(i.sectors[d], storageId)
	}

	return nil
}

func (i *Index) StorageFindSector(ctx context.Context, s abi.SectorID, ft sectorbuilder.SectorFileType) ([]StorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	storageIDs := map[ID]uint64{}

	for _, pathType := range pathTypes {
		if ft&pathType == 0 {
			continue
		}

		for _, id := range i.sectors[Decl{s, pathType}] {
			storageIDs[id]++
		}
	}

	out := make([]StorageInfo, 0, len(storageIDs))

	for id, n := range storageIDs {
		st, ok := i.stores[id]
		if !ok {
			log.Warnf("storage %s is not present in sector index (referenced by sector %v)", id, s)
			continue
		}

		urls := make([]string, len(st.info.URLs))
		for k, u := range st.info.URLs {
			rl, err := url.Parse(u)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse url: %w", err)
			}

			rl.Path = gopath.Join(rl.Path, ft.String(), sectorutil.SectorName(s))
			urls[k] = rl.String()
		}

		out = append(out, StorageInfo{
			ID:       id,
			URLs:     urls,
			Weight:   st.info.Weight * n, // storage with more sector types is better
			CanSeal:  st.info.CanSeal,
			CanStore: st.info.CanStore,
		})
	}

	return out, nil
}

func (i *Index) StorageInfo(ctx context.Context, id ID) (StorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	si, found := i.stores[id]
	if !found {
		return StorageInfo{}, xerrors.Errorf("sector store not found")
	}

	return *si.info, nil
}

func (i *Index) StorageBestAlloc(ctx context.Context, allocate sectorbuilder.SectorFileType, sealing bool) ([]StorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	var candidates []storageEntry

	for _, p := range i.stores {
		if sealing && !p.info.CanSeal {
			log.Debugf("alloc: not considering %s; can't seal", p.info.ID)
			continue
		}
		if !sealing && !p.info.CanStore {
			log.Debugf("alloc: not considering %s; can't store", p.info.ID)
			continue
		}

		// TODO: filter out of space

		candidates = append(candidates, *p)
	}

	if len(candidates) == 0 {
		return nil, xerrors.New("no good path found")
	}

	sort.Slice(candidates, func(i, j int) bool {
		iw := big.Mul(big.NewInt(int64(candidates[i].fsi.Free)), big.NewInt(int64(candidates[i].info.Weight)))
		jw := big.Mul(big.NewInt(int64(candidates[j].fsi.Free)), big.NewInt(int64(candidates[j].info.Weight)))

		return iw.GreaterThan(jw)
	})

	out := make([]StorageInfo, len(candidates))
	for i, candidate := range candidates {
		out[i] = *candidate.info
	}

	return out, nil
}

func (i *Index) FindSector(id abi.SectorID, typ sectorbuilder.SectorFileType) ([]ID, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	return i.sectors[Decl{
		SectorID:       id,
		SectorFileType: typ,
	}], nil
}

var _ SectorIndex = &Index{}
