package stores

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	gopath "path"
	"sort"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/metrics"
)

var HeartbeatInterval = 10 * time.Second
var SkippedHeartbeatThresh = HeartbeatInterval * 5

// ID identifies sector storage by UUID. One sector storage should map to one
//  filesystem, local or networked / shared by multiple machines
type ID string

type Group = string

type StorageInfo struct {
	ID         ID
	URLs       []string // TODO: Support non-http transports
	Weight     uint64
	MaxStorage uint64

	CanSeal  bool
	CanStore bool

	Groups  []Group
	AllowTo []Group
}

type HealthReport struct {
	Stat fsutil.FsStat
	Err  string
}

type SectorStorageInfo struct {
	ID     ID
	URLs   []string // TODO: Support non-http transports
	Weight uint64

	CanSeal  bool
	CanStore bool

	Primary bool
}

//go:generate go run github.com/golang/mock/mockgen -destination=mocks/index.go -package=mocks . SectorIndex

type SectorIndex interface { // part of storage-miner api
	StorageAttach(context.Context, StorageInfo, fsutil.FsStat) error
	StorageInfo(context.Context, ID) (StorageInfo, error)
	StorageReportHealth(context.Context, ID, HealthReport) error

	StorageDeclareSector(ctx context.Context, storageID ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error
	StorageDropSector(ctx context.Context, storageID ID, s abi.SectorID, ft storiface.SectorFileType) error
	StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]SectorStorageInfo, error)

	StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]StorageInfo, error)

	// atomically acquire locks on all sector file types. close ctx to unlock
	StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error
	StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error)

	StorageList(ctx context.Context) (map[ID][]Decl, error)
}

type Decl struct {
	abi.SectorID
	storiface.SectorFileType
}

type declMeta struct {
	storage ID
	primary bool
}

type storageEntry struct {
	info *StorageInfo
	fsi  fsutil.FsStat

	lastHeartbeat time.Time
	heartbeatErr  error
}

type Index struct {
	*indexLocks
	lk sync.RWMutex

	sectors map[Decl][]*declMeta
	stores  map[ID]*storageEntry
}

func NewIndex() *Index {
	return &Index{
		indexLocks: &indexLocks{
			locks: map[abi.SectorID]*sectorLock{},
		},
		sectors: map[Decl][]*declMeta{},
		stores:  map[ID]*storageEntry{},
	}
}

func (i *Index) StorageList(ctx context.Context) (map[ID][]Decl, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	byID := map[ID]map[abi.SectorID]storiface.SectorFileType{}

	for id := range i.stores {
		byID[id] = map[abi.SectorID]storiface.SectorFileType{}
	}
	for decl, ids := range i.sectors {
		for _, id := range ids {
			byID[id.storage][decl.SectorID] |= decl.SectorFileType
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

func (i *Index) StorageAttach(ctx context.Context, si StorageInfo, st fsutil.FsStat) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	log.Infof("New sector storage: %s", si.ID)

	if _, ok := i.stores[si.ID]; ok {
		for _, u := range si.URLs {
			if _, err := url.Parse(u); err != nil {
				return xerrors.Errorf("failed to parse url %s: %w", si.URLs, err)
			}
		}

	uloop:
		for _, u := range si.URLs {
			for _, l := range i.stores[si.ID].info.URLs {
				if u == l {
					continue uloop
				}
			}

			i.stores[si.ID].info.URLs = append(i.stores[si.ID].info.URLs, u)
		}

		i.stores[si.ID].info.Weight = si.Weight
		i.stores[si.ID].info.MaxStorage = si.MaxStorage
		i.stores[si.ID].info.CanSeal = si.CanSeal
		i.stores[si.ID].info.CanStore = si.CanStore
		i.stores[si.ID].info.Groups = si.Groups
		i.stores[si.ID].info.AllowTo = si.AllowTo

		return nil
	}
	i.stores[si.ID] = &storageEntry{
		info: &si,
		fsi:  st,

		lastHeartbeat: time.Now(),
	}
	return nil
}

func (i *Index) StorageReportHealth(ctx context.Context, id ID, report HealthReport) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	ent, ok := i.stores[id]
	if !ok {
		return xerrors.Errorf("health report for unknown storage: %s", id)
	}

	ent.fsi = report.Stat
	if report.Err != "" {
		ent.heartbeatErr = errors.New(report.Err)
	} else {
		ent.heartbeatErr = nil
	}
	ent.lastHeartbeat = time.Now()

	if report.Stat.Capacity > 0 {
		ctx, _ = tag.New(ctx, tag.Upsert(metrics.StorageID, string(id)))

		stats.Record(ctx, metrics.StorageFSAvailable.M(float64(report.Stat.FSAvailable)/float64(report.Stat.Capacity)))
		stats.Record(ctx, metrics.StorageAvailable.M(float64(report.Stat.Available)/float64(report.Stat.Capacity)))
		stats.Record(ctx, metrics.StorageReserved.M(float64(report.Stat.Reserved)/float64(report.Stat.Capacity)))

		stats.Record(ctx, metrics.StorageCapacityBytes.M(report.Stat.Capacity))
		stats.Record(ctx, metrics.StorageFSAvailableBytes.M(report.Stat.FSAvailable))
		stats.Record(ctx, metrics.StorageAvailableBytes.M(report.Stat.Available))
		stats.Record(ctx, metrics.StorageReservedBytes.M(report.Stat.Reserved))

		if report.Stat.Max > 0 {
			stats.Record(ctx, metrics.StorageLimitUsed.M(float64(report.Stat.Used)/float64(report.Stat.Max)))
			stats.Record(ctx, metrics.StorageLimitUsedBytes.M(report.Stat.Used))
			stats.Record(ctx, metrics.StorageLimitMaxBytes.M(report.Stat.Max))
		}
	}

	return nil
}

func (i *Index) StorageDeclareSector(ctx context.Context, storageID ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {
	i.lk.Lock()
	defer i.lk.Unlock()

loop:
	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			continue
		}

		d := Decl{s, fileType}

		for _, sid := range i.sectors[d] {
			if sid.storage == storageID {
				if !sid.primary && primary {
					sid.primary = true
				} else {
					log.Warnf("sector %v redeclared in %s", s, storageID)
				}
				continue loop
			}
		}

		i.sectors[d] = append(i.sectors[d], &declMeta{
			storage: storageID,
			primary: primary,
		})
	}

	return nil
}

func (i *Index) StorageDropSector(ctx context.Context, storageID ID, s abi.SectorID, ft storiface.SectorFileType) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			continue
		}

		d := Decl{s, fileType}

		if len(i.sectors[d]) == 0 {
			continue
		}

		rewritten := make([]*declMeta, 0, len(i.sectors[d])-1)
		for _, sid := range i.sectors[d] {
			if sid.storage == storageID {
				continue
			}

			rewritten = append(rewritten, sid)
		}
		if len(rewritten) == 0 {
			delete(i.sectors, d)
			continue
		}

		i.sectors[d] = rewritten
	}

	return nil
}

func (i *Index) StorageFindSector(ctx context.Context, s abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]SectorStorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	storageIDs := map[ID]uint64{}
	isprimary := map[ID]bool{}

	allowTo := map[Group]struct{}{}

	for _, pathType := range storiface.PathTypes {
		if ft&pathType == 0 {
			continue
		}

		for _, id := range i.sectors[Decl{s, pathType}] {
			storageIDs[id.storage]++
			isprimary[id.storage] = isprimary[id.storage] || id.primary
		}
	}

	out := make([]SectorStorageInfo, 0, len(storageIDs))

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

			rl.Path = gopath.Join(rl.Path, ft.String(), storiface.SectorName(s))
			urls[k] = rl.String()
		}

		if allowTo != nil && len(st.info.AllowTo) > 0 {
			for _, group := range st.info.AllowTo {
				allowTo[group] = struct{}{}
			}
		} else {
			allowTo = nil // allow to any
		}

		out = append(out, SectorStorageInfo{
			ID:     id,
			URLs:   urls,
			Weight: st.info.Weight * n, // storage with more sector types is better

			CanSeal:  st.info.CanSeal,
			CanStore: st.info.CanStore,

			Primary: isprimary[id],
		})
	}

	if allowFetch {
		spaceReq, err := ft.SealSpaceUse(ssize)
		if err != nil {
			return nil, xerrors.Errorf("estimating required space: %w", err)
		}

		for id, st := range i.stores {
			if !st.info.CanSeal {
				continue
			}

			if spaceReq > uint64(st.fsi.Available) {
				log.Debugf("not selecting on %s, out of space (available: %d, need: %d)", st.info.ID, st.fsi.Available, spaceReq)
				continue
			}

			if time.Since(st.lastHeartbeat) > SkippedHeartbeatThresh {
				log.Debugf("not selecting on %s, didn't receive heartbeats for %s", st.info.ID, time.Since(st.lastHeartbeat))
				continue
			}

			if st.heartbeatErr != nil {
				log.Debugf("not selecting on %s, heartbeat error: %s", st.info.ID, st.heartbeatErr)
				continue
			}

			if _, ok := storageIDs[id]; ok {
				continue
			}

			if allowTo != nil {
				allow := false
				for _, group := range st.info.Groups {
					if _, found := allowTo[group]; found {
						log.Debugf("path %s in allowed group %s", st.info.ID, group)
						allow = true
						break
					}
				}

				if !allow {
					log.Debugf("not selecting on %s, not in allowed group, allow %+v; path has %+v", st.info.ID, allowTo, st.info.Groups)
					continue
				}
			}

			urls := make([]string, len(st.info.URLs))
			for k, u := range st.info.URLs {
				rl, err := url.Parse(u)
				if err != nil {
					return nil, xerrors.Errorf("failed to parse url: %w", err)
				}

				rl.Path = gopath.Join(rl.Path, ft.String(), storiface.SectorName(s))
				urls[k] = rl.String()
			}

			out = append(out, SectorStorageInfo{
				ID:     id,
				URLs:   urls,
				Weight: st.info.Weight * 0, // TODO: something better than just '0'

				CanSeal:  st.info.CanSeal,
				CanStore: st.info.CanStore,

				Primary: false,
			})
		}
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

func (i *Index) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]StorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	var candidates []storageEntry

	var err error
	var spaceReq uint64
	switch pathType {
	case storiface.PathSealing:
		spaceReq, err = allocate.SealSpaceUse(ssize)
	case storiface.PathStorage:
		spaceReq, err = allocate.StoreSpaceUse(ssize)
	default:
		panic(fmt.Sprintf("unexpected pathType: %s", pathType))
	}
	if err != nil {
		return nil, xerrors.Errorf("estimating required space: %w", err)
	}

	for _, p := range i.stores {
		if (pathType == storiface.PathSealing) && !p.info.CanSeal {
			continue
		}
		if (pathType == storiface.PathStorage) && !p.info.CanStore {
			continue
		}

		if spaceReq > uint64(p.fsi.Available) {
			log.Debugf("not allocating on %s, out of space (available: %d, need: %d)", p.info.ID, p.fsi.Available, spaceReq)
			continue
		}

		if time.Since(p.lastHeartbeat) > SkippedHeartbeatThresh {
			log.Debugf("not allocating on %s, didn't receive heartbeats for %s", p.info.ID, time.Since(p.lastHeartbeat))
			continue
		}

		if p.heartbeatErr != nil {
			log.Debugf("not allocating on %s, heartbeat error: %s", p.info.ID, p.heartbeatErr)
			continue
		}

		candidates = append(candidates, *p)
	}

	if len(candidates) == 0 {
		return nil, xerrors.New("no good path found")
	}

	sort.Slice(candidates, func(i, j int) bool {
		iw := big.Mul(big.NewInt(candidates[i].fsi.Available), big.NewInt(int64(candidates[i].info.Weight)))
		jw := big.Mul(big.NewInt(candidates[j].fsi.Available), big.NewInt(int64(candidates[j].info.Weight)))

		return iw.GreaterThan(jw)
	})

	out := make([]StorageInfo, len(candidates))
	for i, candidate := range candidates {
		out[i] = *candidate.info
	}

	return out, nil
}

func (i *Index) FindSector(id abi.SectorID, typ storiface.SectorFileType) ([]ID, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	f, ok := i.sectors[Decl{
		SectorID:       id,
		SectorFileType: typ,
	}]
	if !ok {
		return nil, nil
	}
	out := make([]ID, 0, len(f))
	for _, meta := range f {
		out = append(out, meta.storage)
	}

	return out, nil
}

var _ SectorIndex = &Index{}
