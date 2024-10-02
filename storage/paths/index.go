package paths

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

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var HeartbeatInterval = 10 * time.Second
var SkippedHeartbeatThresh = HeartbeatInterval * 5

type SectorIndex interface { // part of storage-miner api
	StorageAttach(context.Context, storiface.StorageInfo, fsutil.FsStat) error
	StorageDetach(ctx context.Context, id storiface.ID, url string) error
	StorageInfo(context.Context, storiface.ID) (storiface.StorageInfo, error)
	StorageReportHealth(context.Context, storiface.ID, storiface.HealthReport) error

	StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error
	StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error
	StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error)

	StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType, miner abi.ActorID) ([]storiface.StorageInfo, error)

	// atomically acquire locks on all sector file types. close ctx to unlock
	StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error
	StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error)
	StorageGetLocks(ctx context.Context) (storiface.SectorLocks, error)

	StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error)
}

type declMeta struct {
	storage storiface.ID
	primary bool
}

type storageEntry struct {
	info *storiface.StorageInfo
	fsi  fsutil.FsStat

	lastHeartbeat time.Time
	heartbeatErr  error
}

// MemIndex represents an in-memory index of storage sectors and storage entries.
type MemIndex struct {
	*indexLocks
	lk sync.RWMutex

	// optional
	alerting   *alerting.Alerting
	pathAlerts map[storiface.ID]alerting.AlertType

	sectors map[storiface.Decl][]*declMeta
	stores  map[storiface.ID]*storageEntry
}

func NewMemIndex(al *alerting.Alerting) *MemIndex {
	return &MemIndex{
		indexLocks: &indexLocks{
			locks: map[abi.SectorID]*sectorLock{},
		},

		alerting:   al,
		pathAlerts: map[storiface.ID]alerting.AlertType{},

		sectors: map[storiface.Decl][]*declMeta{},
		stores:  map[storiface.ID]*storageEntry{},
	}
}

func (i *MemIndex) StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	byID := map[storiface.ID]map[abi.SectorID]storiface.SectorFileType{}

	for id := range i.stores {
		byID[id] = map[abi.SectorID]storiface.SectorFileType{}
	}
	for decl, ids := range i.sectors {
		for _, id := range ids {
			byID[id.storage][decl.SectorID] |= decl.SectorFileType
		}
	}

	out := map[storiface.ID][]storiface.Decl{}
	for id, m := range byID {
		out[id] = []storiface.Decl{}
		for sectorID, fileType := range m {
			out[id] = append(out[id], storiface.Decl{
				SectorID:       sectorID,
				SectorFileType: fileType,
			})
		}
	}

	return out, nil
}

func (i *MemIndex) StorageAttach(ctx context.Context, si storiface.StorageInfo, st fsutil.FsStat) error {
	var allow, deny = make([]string, 0, len(si.AllowTypes)), make([]string, 0, len(si.DenyTypes))

	if _, hasAlert := i.pathAlerts[si.ID]; i.alerting != nil && !hasAlert {
		i.pathAlerts[si.ID] = i.alerting.AddAlertType("sector-index", "pathconf-"+string(si.ID))
	}

	var hasConfigIssues bool

	for id, typ := range si.AllowTypes {
		_, err := storiface.TypeFromString(typ)
		if err != nil {
			// No need to hard-fail here, just warn the user
			// (note that even with all-invalid entries we'll deny all types, so nothing unexpected should enter the path)
			hasConfigIssues = true

			if i.alerting != nil {
				i.alerting.Raise(i.pathAlerts[si.ID], map[string]interface{}{
					"message":   "bad path type in AllowTypes",
					"path":      string(si.ID),
					"idx":       id,
					"path_type": typ,
					"error":     err.Error(),
				})
			}

			continue
		}
		allow = append(allow, typ)
	}
	for id, typ := range si.DenyTypes {
		_, err := storiface.TypeFromString(typ)
		if err != nil {
			// No need to hard-fail here, just warn the user
			hasConfigIssues = true

			if i.alerting != nil {
				i.alerting.Raise(i.pathAlerts[si.ID], map[string]interface{}{
					"message":   "bad path type in DenyTypes",
					"path":      string(si.ID),
					"idx":       id,
					"path_type": typ,
					"error":     err.Error(),
				})
			}

			continue
		}
		deny = append(deny, typ)
	}
	si.AllowTypes = allow
	si.DenyTypes = deny

	if i.alerting != nil && !hasConfigIssues && i.alerting.IsRaised(i.pathAlerts[si.ID]) {
		i.alerting.Resolve(i.pathAlerts[si.ID], map[string]string{
			"message": "path config is now correct",
		})
	}

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
		i.stores[si.ID].info.AllowTypes = allow
		i.stores[si.ID].info.DenyTypes = deny
		i.stores[si.ID].info.AllowMiners = si.AllowMiners
		i.stores[si.ID].info.DenyMiners = si.DenyMiners

		return nil
	}
	i.stores[si.ID] = &storageEntry{
		info: &si,
		fsi:  st,

		lastHeartbeat: time.Now(),
	}

	return nil
}

func (i *MemIndex) StorageDetach(ctx context.Context, id storiface.ID, url string) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	// ent: *storageEntry
	ent, ok := i.stores[id]
	if !ok {
		return xerrors.Errorf("storage '%s' isn't registered", id)
	}

	// check if this is the only path provider/url for this pathID
	drop := true
	if len(ent.info.URLs) > 0 {
		drop = len(ent.info.URLs) == 1 // only one url

		if drop && ent.info.URLs[0] != url {
			return xerrors.Errorf("not dropping path, requested and index urls don't match ('%s' != '%s')", url, ent.info.URLs[0])
		}
	}

	if drop {
		if a, hasAlert := i.pathAlerts[id]; hasAlert && i.alerting != nil {
			if i.alerting.IsRaised(a) {
				i.alerting.Resolve(a, map[string]string{
					"message": "path detached",
				})
			}
			delete(i.pathAlerts, id)
		}

		// stats
		var droppedEntries, primaryEntries, droppedDecls int

		// drop declarations
		for decl, dms := range i.sectors {
			var match bool
			for _, dm := range dms {
				if dm.storage == id {
					match = true
					droppedEntries++
					if dm.primary {
						primaryEntries++
					}
					break
				}
			}

			// if no entries match, nothing to do here
			if !match {
				continue
			}

			// if there's a match, and only one entry, drop the whole declaration
			if len(dms) <= 1 {
				delete(i.sectors, decl)
				droppedDecls++
				continue
			}

			// rewrite entries with the path we're dropping filtered out
			filtered := make([]*declMeta, 0, len(dms)-1)
			for _, dm := range dms {
				if dm.storage != id {
					filtered = append(filtered, dm)
				}
			}

			i.sectors[decl] = filtered
		}

		delete(i.stores, id)

		log.Warnw("Dropping sector storage", "path", id, "url", url, "droppedEntries", droppedEntries, "droppedPrimaryEntries", primaryEntries, "droppedDecls", droppedDecls)
	} else {
		newUrls := make([]string, 0, len(ent.info.URLs))
		for _, u := range ent.info.URLs {
			if u != url {
				newUrls = append(newUrls, u)
			}
		}
		ent.info.URLs = newUrls

		log.Warnw("Dropping sector path endpoint", "path", id, "url", url)
	}

	return nil
}

func (i *MemIndex) StorageReportHealth(ctx context.Context, id storiface.ID, report storiface.HealthReport) error {
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
		ctx, _ = tag.New(ctx,
			tag.Upsert(metrics.StorageID, string(id)),
			tag.Upsert(metrics.PathStorage, fmt.Sprint(ent.info.CanStore)),
			tag.Upsert(metrics.PathSeal, fmt.Sprint(ent.info.CanSeal)),
		)

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

func (i *MemIndex) StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {
	i.lk.Lock()
	defer i.lk.Unlock()

loop:
	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			continue
		}

		d := storiface.Decl{SectorID: s, SectorFileType: fileType}

		for _, sid := range i.sectors[d] {
			if sid.storage == storageID {
				if !sid.primary && primary {
					sid.primary = true
				} else {
					log.Debugf("sector %v redeclared in %s", s, storageID)
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

func (i *MemIndex) StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error {
	i.lk.Lock()
	defer i.lk.Unlock()

	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			continue
		}

		d := storiface.Decl{SectorID: s, SectorFileType: fileType}

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

func (i *MemIndex) StorageFindSector(ctx context.Context, s abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	storageIDs := map[storiface.ID]uint64{}
	isprimary := map[storiface.ID]bool{}

	allowTo := map[storiface.Group]struct{}{}

	for _, pathType := range storiface.PathTypes {
		if ft&pathType == 0 {
			continue
		}

		for _, id := range i.sectors[storiface.Decl{SectorID: s, SectorFileType: pathType}] {
			storageIDs[id.storage]++
			isprimary[id.storage] = isprimary[id.storage] || id.primary
		}
	}

	out := make([]storiface.SectorStorageInfo, 0, len(storageIDs))

	for id, n := range storageIDs {
		st, ok := i.stores[id]
		if !ok {
			log.Warnf("storage %s is not present in sector index (referenced by sector %v)", id, s)
			continue
		}

		urls, burls := make([]string, len(st.info.URLs)), make([]string, len(st.info.URLs))
		for k, u := range st.info.URLs {
			rl, err := url.Parse(u)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse url: %w", err)
			}

			rl.Path = gopath.Join(rl.Path, ft.String(), storiface.SectorName(s))
			urls[k] = rl.String()
			burls[k] = u
		}

		if allowTo != nil && len(st.info.AllowTo) > 0 {
			for _, group := range st.info.AllowTo {
				allowTo[group] = struct{}{}
			}
		} else {
			allowTo = nil // allow to any
		}

		out = append(out, storiface.SectorStorageInfo{
			ID:       id,
			URLs:     urls,
			BaseURLs: burls,
			Weight:   st.info.Weight * n, // storage with more sector types is better

			CanSeal:  st.info.CanSeal,
			CanStore: st.info.CanStore,

			Primary: isprimary[id],

			AllowTypes:  st.info.AllowTypes,
			DenyTypes:   st.info.DenyTypes,
			AllowMiners: st.info.AllowMiners,
			DenyMiners:  st.info.DenyMiners,
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

			proceed, msg, err := MinerFilter(st.info.AllowMiners, st.info.DenyMiners, s.Miner)
			if err != nil {
				return nil, err
			}

			if !proceed {
				log.Debugf("not allocating on %s, miner %s %s", st.info.ID, s.Miner.String(), msg)
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

			if !ft.AnyAllowed(st.info.AllowTypes, st.info.DenyTypes) {
				log.Debugf("not selecting on %s, not allowed by file type filters", st.info.ID)
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

			urls, burls := make([]string, len(st.info.URLs)), make([]string, len(st.info.URLs))
			for k, u := range st.info.URLs {
				rl, err := url.Parse(u)
				if err != nil {
					return nil, xerrors.Errorf("failed to parse url: %w", err)
				}

				rl.Path = gopath.Join(rl.Path, ft.String(), storiface.SectorName(s))
				urls[k] = rl.String()
				burls[k] = u
			}

			out = append(out, storiface.SectorStorageInfo{
				ID:       id,
				URLs:     urls,
				BaseURLs: burls,
				Weight:   st.info.Weight * 0, // TODO: something better than just '0'

				CanSeal:  st.info.CanSeal,
				CanStore: st.info.CanStore,

				Primary: false,

				AllowTypes:  st.info.AllowTypes,
				DenyTypes:   st.info.DenyTypes,
				AllowMiners: st.info.AllowMiners,
				DenyMiners:  st.info.DenyMiners,
			})
		}
	}

	return out, nil
}

// StorageInfo retrieves the storage information for a given storage ID.
//
// The method first acquires a read lock on the MemIndex to ensure thread-safety.
// It then checks if the storage ID exists in the stores map. If not, it returns
// an error indicating that the sector store was not found.
//
// Finally, it returns the storage information of the selected storage.
//
// Parameters:
// - ctx: the context.Context object for cancellation and timeouts
// - id: the ID of the storage to retrieve information for
//
// Returns:
// - storiface.StorageInfo: the storage information of the selected storage ID
// - error: an error indicating any issues encountered during the process
func (i *MemIndex) StorageInfo(ctx context.Context, id storiface.ID) (storiface.StorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	si, found := i.stores[id]
	if !found {
		return storiface.StorageInfo{}, xerrors.Errorf("sector store not found")
	}

	return *si.info, nil
}

// StorageBestAlloc selects the best available storage options for allocating
// a sector file. It takes into account the allocation type (sealing or storage),
// sector size, and path type (sealing or storage).
//
// The method first estimates the required space for the allocation based on the
// sector size and path type. It then iterates through all available storage options
// and filters out those that cannot be used for the given path type. It also filters
// out storage options that do not have enough available space or have not received
// heartbeats within a certain threshold.
//
// The remaining storage options are sorted based on their available space and weight,
// with higher availability and weight being prioritized. The method then returns
// the information of the selected storage options.
//
// If no suitable storage options are found, it returns an error indicating that
// no good path is available.
//
// Parameters:
// - ctx: the context.Context object for cancellation and timeouts
// - allocate: the type of allocation (sealing or storage)
// - ssize: the size of the sector file
// - pathType: the path type (sealing or storage)
//
// Returns:
// - []storiface.StorageInfo: the information of the selected storage options
// - error: an error indicating any issues encountered during the process
func (i *MemIndex) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType, miner abi.ActorID) ([]storiface.StorageInfo, error) {
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

		proceed, msg, err := MinerFilter(p.info.AllowMiners, p.info.DenyMiners, miner)
		if err != nil {
			return nil, err
		}

		if !proceed {
			log.Debugf("not allocating on %s, miner %s %s", p.info.ID, miner.String(), msg)
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

	out := make([]storiface.StorageInfo, len(candidates))
	for i, candidate := range candidates {
		out[i] = *candidate.info
	}

	return out, nil
}

func (i *MemIndex) FindSector(id abi.SectorID, typ storiface.SectorFileType) ([]storiface.ID, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	f, ok := i.sectors[storiface.Decl{
		SectorID:       id,
		SectorFileType: typ,
	}]
	if !ok {
		return nil, nil
	}
	out := make([]storiface.ID, 0, len(f))
	for _, meta := range f {
		out = append(out, meta.storage)
	}

	return out, nil
}

var _ SectorIndex = &MemIndex{}

func MinerFilter(allowMiners, denyMiners []string, miner abi.ActorID) (bool, string, error) {
	checkMinerInList := func(minersList []string, miner abi.ActorID) (bool, error) {
		for _, m := range minersList {
			minerIDStr := m
			maddr, err := address.NewFromString(minerIDStr)
			if err != nil {
				return false, xerrors.Errorf("parsing miner address: %w", err)
			}
			mid, err := address.IDFromAddress(maddr)
			if err != nil {
				return false, xerrors.Errorf("converting miner address to ID: %w", err)
			}
			if abi.ActorID(mid) == miner {
				return true, nil
			}
		}
		return false, nil
	}

	if len(allowMiners) > 0 {
		found, err := checkMinerInList(allowMiners, miner)
		if err != nil || !found {
			return false, "not allowed", err
		}
	}

	if len(denyMiners) > 0 {
		found, err := checkMinerInList(denyMiners, miner)
		if err != nil || found {
			return false, "denied", err
		}
	}
	return true, "", nil
}
