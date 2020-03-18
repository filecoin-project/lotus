package stores

import (
	"context"
	"net/url"
	gopath "path"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
)

// ID identifies sector storage by UUID. One sector storage should map to one
//  filesystem, local or networked / shared by multiple machines
type ID string

type StorageInfo struct {
	ID   ID
	URLs []string // TODO: Support non-http transports
	Cost int

	CanSeal  bool
	CanStore bool
}

type SectorIndex interface { // part of storage-miner api
	StorageAttach(context.Context, StorageInfo) error

	StorageDeclareSector(ctx context.Context, storageId ID, s abi.SectorID, ft sectorbuilder.SectorFileType) error
	StorageFindSector(context.Context, abi.SectorID, sectorbuilder.SectorFileType) ([]StorageInfo, error)
}

type Decl struct {
	abi.SectorID
	sectorbuilder.SectorFileType
}

type Index struct {
	lk sync.Mutex

	sectors map[Decl][]ID
	stores  map[ID]*StorageInfo
}

func NewIndex() *Index {
	return &Index{
		sectors: map[Decl][]ID{},
		stores:  map[ID]*StorageInfo{},
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

func (i *Index) StorageAttach(ctx context.Context, si StorageInfo) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	log.Infof("New sector storage: %s", si.ID)

	if _, ok := i.stores[si.ID]; ok {
		for _, u := range si.URLs {
			if _, err := url.Parse(u); err != nil {
				return xerrors.Errorf("failed to parse url %s: %w", si.URLs, err)
			}
		}

		i.stores[si.ID].URLs = append(i.stores[si.ID].URLs, si.URLs...)
		return nil
	}
	i.stores[si.ID] = &si
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
	i.lk.Lock()
	defer i.lk.Unlock()

	storageIDs := i.sectors[Decl{s, ft}]
	out := make([]StorageInfo, len(storageIDs))

	for j, id := range storageIDs {
		st, ok := i.stores[id]
		if !ok {
			log.Warnf("storage %s is not present in sector index (referenced by sector %v)", id, s)
			continue
		}

		urls := make([]string, len(st.URLs))
		for k, u := range st.URLs {
			rl, err := url.Parse(u)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse url: %w", err)
			}

			rl.Path = gopath.Join(rl.Path, ft.String(), sectorutil.SectorName(s))
			urls[k] = rl.String()
		}

		out[j] = StorageInfo{
			ID:       id,
			URLs:     nil,
			Cost:     st.Cost,
			CanSeal:  st.CanSeal,
			CanStore: st.CanStore,
		}
	}

	return out, nil
}

func (i *Index) StorageInfo(ctx context.Context, id ID) (StorageInfo, error) {
	si, found := i.stores[id]
	if !found {
		return StorageInfo{}, xerrors.Errorf("sector store not found")
	}

	return *si, nil
}

func DeclareLocalStorage(ctx context.Context, idx SectorIndex, localStore *Local, urls []string, cost int) error {
	for _, path := range localStore.Local() {
		err := idx.StorageAttach(ctx, StorageInfo{
			ID:       path.ID,
			URLs:     urls,
			Cost:     cost,
			CanSeal:  path.CanSeal,
			CanStore: path.CanStore,
		})
		if err != nil {
			log.Errorf("attaching local storage to remote: %+v", err)
			continue
		}

		for id, fileType := range localStore.List(path.ID) {
			if err := idx.StorageDeclareSector(ctx, path.ID, id, fileType); err != nil {
				log.Errorf("declaring sector: %+v")
			}
		}
	}

	return nil
}

var _ SectorIndex = &Index{}
