package paths

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	gopath "path"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var HeartbeatInterval = 10 * time.Second
var SkippedHeartbeatThresh = HeartbeatInterval * 5

//go:generate go run github.com/golang/mock/mockgen -destination=mocks/index.go -package=mocks . SectorIndex

type SectorIndex interface { // part of storage-miner api
	StorageAttach(context.Context, storiface.StorageInfo, fsutil.FsStat) error
	StorageDetach(ctx context.Context, id storiface.ID, url string) error
	StorageInfo(context.Context, storiface.ID) (storiface.StorageInfo, error)
	StorageReportHealth(context.Context, storiface.ID, storiface.HealthReport) error

	StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error
	StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error
	StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error)

	StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]storiface.StorageInfo, error)

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

type IndexProxy struct {
	memIndex            *Index
	dbIndex             *DBIndex
	enableSectorIndexDB bool
}

func (ip *IndexProxy) StorageAttach(ctx context.Context, info storiface.StorageInfo, stat fsutil.FsStat) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageAttach(ctx, info, stat)
	}
	return ip.memIndex.StorageAttach(ctx, info, stat)
}

func (ip *IndexProxy) StorageDetach(ctx context.Context, id storiface.ID, url string) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageDetach(ctx, id, url)
	}
	return ip.memIndex.StorageDetach(ctx, id, url)
}

func (ip *IndexProxy) StorageInfo(ctx context.Context, id storiface.ID) (storiface.StorageInfo, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageInfo(ctx, id)
	}
	return ip.memIndex.StorageInfo(ctx, id)
}

func (ip *IndexProxy) StorageReportHealth(ctx context.Context, id storiface.ID, report storiface.HealthReport) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageReportHealth(ctx, id, report)
	}
	return ip.memIndex.StorageReportHealth(ctx, id, report)
}

func (ip *IndexProxy) StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageDeclareSector(ctx, storageID, s, ft, primary)
	}
	return ip.memIndex.StorageDeclareSector(ctx, storageID, s, ft, primary)
}

func (ip *IndexProxy) StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageDropSector(ctx, storageID, s, ft)
	}
	return ip.memIndex.StorageDropSector(ctx, storageID, s, ft)
}

func (ip *IndexProxy) StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageFindSector(ctx, sector, ft, ssize, allowFetch)
	}
	return ip.memIndex.StorageFindSector(ctx, sector, ft, ssize, allowFetch)
}

func (ip *IndexProxy) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]storiface.StorageInfo, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageBestAlloc(ctx, allocate, ssize, pathType)
	}
	return ip.memIndex.StorageBestAlloc(ctx, allocate, ssize, pathType)
}

func (ip *IndexProxy) StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error {
	return ip.memIndex.StorageLock(ctx, sector, read, write)
}

func (ip *IndexProxy) StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error) {
	return ip.memIndex.StorageTryLock(ctx, sector, read, write)
}

func (ip *IndexProxy) StorageGetLocks(ctx context.Context) (storiface.SectorLocks, error) {
	return ip.memIndex.StorageGetLocks(ctx)
}

func (ip *IndexProxy) StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error) {
	if ip.enableSectorIndexDB {
		return ip.dbIndex.StorageList(ctx)
	}
	return ip.memIndex.StorageList(ctx)
}

type Index struct {
	*indexLocks
	lk sync.RWMutex

	// optional
	alerting   *alerting.Alerting
	pathAlerts map[storiface.ID]alerting.AlertType

	sectors map[storiface.Decl][]*declMeta
	stores  map[storiface.ID]*storageEntry
}

type DBIndex struct {
	alerting   *alerting.Alerting
	pathAlerts map[storiface.ID]alerting.AlertType

	harmonyDB *harmonydb.DB
}

func NewIndexProxyHelper(enableSectorIndexDB bool) func(al *alerting.Alerting, db *harmonydb.DB) *IndexProxy {
	return func(al *alerting.Alerting, db *harmonydb.DB) *IndexProxy {
		return NewIndexProxy(al, db, enableSectorIndexDB)
	}
}

func NewIndexProxy(al *alerting.Alerting, db *harmonydb.DB, enableSectorIndexDB bool) *IndexProxy {
	return &IndexProxy{
		memIndex:            NewIndex(al),
		dbIndex:             NewDBIndex(al, db),
		enableSectorIndexDB: enableSectorIndexDB,
	}
}

func NewDBIndex(al *alerting.Alerting, db *harmonydb.DB) *DBIndex {
	return &DBIndex{
		harmonyDB: db,

		alerting:   al,
		pathAlerts: map[storiface.ID]alerting.AlertType{},
	}
}

func NewIndex(al *alerting.Alerting) *Index {
	return &Index{
		indexLocks: &indexLocks{
			locks: map[abi.SectorID]*sectorLock{},
		},

		alerting:   al,
		pathAlerts: map[storiface.ID]alerting.AlertType{},

		sectors: map[storiface.Decl][]*declMeta{},
		stores:  map[storiface.ID]*storageEntry{},
	}
}

func (dbi *DBIndex) StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error) {

	var sectorEntries []struct {
		Storage_id      string
		Miner_id        sql.NullInt64
		Sector_num      sql.NullInt64
		Sector_filetype sql.NullInt32
		Is_primary      sql.NullBool
	}

	err := dbi.harmonyDB.Select(ctx, &sectorEntries,
		"select stor.storage_id, miner_id, sector_num, sector_filetype, is_primary from storagelocation stor left join sectorlocation sec on stor.storage_id=sec.storage_id")
	if err != nil {
		return nil, xerrors.Errorf("StorageList DB query fails: ", err)
	}

	log.Errorf("Sector entries: ", sectorEntries)

	byID := map[storiface.ID]map[abi.SectorID]storiface.SectorFileType{}
	for _, entry := range sectorEntries {
		id := storiface.ID(entry.Storage_id)
		_, ok := byID[id]
		if !ok {
			byID[id] = map[abi.SectorID]storiface.SectorFileType{}
		}

		// skip sector info for storage paths with no sectors
		if !entry.Miner_id.Valid {
			continue
		}

		sectorId := abi.SectorID{
			Miner:  abi.ActorID(entry.Miner_id.Int64),
			Number: abi.SectorNumber(entry.Sector_num.Int64),
		}

		byID[id][sectorId] |= storiface.SectorFileType(entry.Sector_filetype.Int32)
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

func (i *Index) StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error) {
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

func union(a, b []string) []string {
	m := make(map[string]bool)

	for _, elem := range a {
		m[elem] = true
	}

	for _, elem := range b {
		if _, ok := m[elem]; !ok {
			a = append(a, elem)
		}
	}
	return a
}

func splitString(str string) []string {
	if str == "" {
		return []string{}
	}
	return strings.Split(str, ",")
}

func (dbi *DBIndex) StorageAttach(ctx context.Context, si storiface.StorageInfo, st fsutil.FsStat) error {
	var allow, deny = make([]string, 0, len(si.AllowTypes)), make([]string, 0, len(si.DenyTypes))

	if _, hasAlert := dbi.pathAlerts[si.ID]; dbi.alerting != nil && !hasAlert {
		dbi.pathAlerts[si.ID] = dbi.alerting.AddAlertType("sector-index", "pathconf-"+string(si.ID))
	}

	var hasConfigIssues bool

	for id, typ := range si.AllowTypes {
		_, err := storiface.TypeFromString(typ)
		if err != nil {
			//No need to hard-fail here, just warn the user
			//(note that even with all-invalid entries we'll deny all types, so nothing unexpected should enter the path)
			hasConfigIssues = true

			if dbi.alerting != nil {
				dbi.alerting.Raise(dbi.pathAlerts[si.ID], map[string]interface{}{
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
			//No need to hard-fail here, just warn the user
			hasConfigIssues = true

			if dbi.alerting != nil {
				dbi.alerting.Raise(dbi.pathAlerts[si.ID], map[string]interface{}{
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

	if dbi.alerting != nil && !hasConfigIssues && dbi.alerting.IsRaised(dbi.pathAlerts[si.ID]) {
		dbi.alerting.Resolve(dbi.pathAlerts[si.ID], map[string]string{
			"message": "path config is now correct",
		})
	}

	for _, u := range si.URLs {
		if _, err := url.Parse(u); err != nil {
			return xerrors.Errorf("failed to parse url %s: %w", si.URLs, err)
		}
	}

	// TODO: make below part of a single transaction
	var urls sql.NullString
	var storageId sql.NullString
	err := dbi.harmonyDB.QueryRow(ctx,
		"Select storage_id, urls from StorageLocation where storage_id = $1", string(si.ID)).Scan(&storageId, &urls)
	if err != nil && !strings.Contains(err.Error(), "no rows in result set") {
		return xerrors.Errorf("storage attach select fails: ", err)
	}

	// Storage ID entry exists
	if storageId.Valid {
		var currUrls []string
		if urls.Valid {
			currUrls = strings.Split(urls.String, ",")
		}
		currUrls = union(currUrls, si.URLs)

		_, err = dbi.harmonyDB.Exec(ctx,
			"update StorageLocation set urls=$1, weight=$2, max_storage=$3, can_seal=$4, can_store=$5, groups=$6, allow_to=$7, allow_types=$8, deny_types=$9 where storage_id=$10",
			strings.Join(currUrls, ","),
			si.Weight,
			si.MaxStorage,
			si.CanSeal,
			si.CanStore,
			strings.Join(si.Groups, ","),
			strings.Join(si.AllowTo, ","),
			strings.Join(si.AllowTypes, ","),
			strings.Join(si.DenyTypes, ","),
			si.ID)
		if err != nil {
			return xerrors.Errorf("storage attach update fails: ", err)
		}

		return nil
	}

	// Insert storage id
	_, err = dbi.harmonyDB.Exec(ctx,
		"insert into StorageLocation "+
			"Values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)",
		si.ID,
		strings.Join(si.URLs, ","),
		si.Weight,
		si.MaxStorage,
		si.CanSeal,
		si.CanStore,
		strings.Join(si.Groups, ","),
		strings.Join(si.AllowTo, ","),
		strings.Join(si.AllowTypes, ","),
		strings.Join(si.DenyTypes, ","),
		st.Capacity,
		st.Available,
		st.FSAvailable,
		st.Reserved,
		st.Used,
		time.Now())
	if err != nil {
		log.Errorf("StorageAttach insert fails: ", err)
		return xerrors.Errorf("StorageAttach insert fails: ", err)
	}
	return nil
}

func (i *Index) StorageAttach(ctx context.Context, si storiface.StorageInfo, st fsutil.FsStat) error {
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

		return nil
	}
	i.stores[si.ID] = &storageEntry{
		info: &si,
		fsi:  st,

		lastHeartbeat: time.Now(),
	}

	return nil
}

func (dbi *DBIndex) StorageDetach(ctx context.Context, id storiface.ID, url string) error {

	// If url not in path urls, error out
	// if this is only path url for this storage path, drop storage path and sector decls which have this as a storage path

	var qUrls sql.NullString
	err := dbi.harmonyDB.QueryRow(ctx, "select urls from storagelocation where storage_id=$1", string(id)).Scan(&qUrls)
	if err != nil {
		return err
	}

	if !qUrls.Valid {
		return xerrors.Errorf("No urls available for storage id: ", id)
	}
	urls := splitString(qUrls.String)

	var modUrls []string
	for _, u := range urls {
		if u != url {
			modUrls = append(modUrls, u)
		}
	}

	// noop if url doesn't exist in urls
	if len(modUrls) == len(urls) {
		return nil
	}

	if len(modUrls) > 0 {
		newUrls := strings.Join(modUrls, ",")
		_, err := dbi.harmonyDB.Exec(ctx, "update storagelocation set urls=$1 where storage_id=$2", newUrls, id)
		if err != nil {
			return err
		}

		log.Warnw("Dropping sector path endpoint", "path", id, "url", url)
	} else {
		// Todo: single transaction

		// Drop storage path completely
		_, err := dbi.harmonyDB.Exec(ctx, "delete from storagelocation where storage_id=$1", id)
		if err != nil {
			return err
		}

		// Drop all sectors entries which use this storage path
		_, err = dbi.harmonyDB.Exec(ctx, "delete from sectorlocation where storage_id=$1", id)
		if err != nil {
			return err
		}
		log.Warnw("Dropping sector storage", "path", id)
	}

	return nil
}

func (i *Index) StorageDetach(ctx context.Context, id storiface.ID, url string) error {
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

func (dbi *DBIndex) StorageReportHealth(ctx context.Context, id storiface.ID, report storiface.HealthReport) error {

	var canSeal, canStore bool
	err := dbi.harmonyDB.QueryRow(ctx,
		"select can_seal, can_store from storagelocation where storage_id=$1", id).Scan(&canSeal, &canStore)
	if err != nil {
		return xerrors.Errorf("Querying for storage id %d fails with err %w", id, err)
	}

	_, err = dbi.harmonyDB.Exec(ctx,
		"update storagelocation set capacity=$1, available=$2, fs_available=$3, reserved=$4, used=$5, last_heartbeat=$6",
		report.Stat.Capacity,
		report.Stat.Available,
		report.Stat.FSAvailable,
		report.Stat.Reserved,
		report.Stat.Used,
		time.Now())
	if err != nil {
		return xerrors.Errorf("updating storage health in DB fails with err: ", err)
	}

	if report.Stat.Capacity > 0 {
		ctx, _ = tag.New(ctx,
			tag.Upsert(metrics.StorageID, string(id)),
			tag.Upsert(metrics.PathStorage, fmt.Sprint(canStore)),
			tag.Upsert(metrics.PathSeal, fmt.Sprint(canSeal)),
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

func (i *Index) StorageReportHealth(ctx context.Context, id storiface.ID, report storiface.HealthReport) error {
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

func (dbi *DBIndex) StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {

	ftValid := false
	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			ftValid = true
			break
		}
	}
	if !ftValid {
		return xerrors.Errorf("Invalid filetype")
	}

	// TODO: make the belowpart of a single transaction
	var currPrimary sql.NullBool
	err := dbi.harmonyDB.QueryRow(ctx,
		"select is_primary from SectorLocation where miner_id=$1 and sector_num=$2 and sector_filetype=$3 and storage_id=$4",
		uint64(s.Miner), uint64(s.Number), int(ft), string(storageID)).Scan(&currPrimary)
	if err != nil && !strings.Contains(err.Error(), "no rows in result set") {
		return xerrors.Errorf("DB select fails: ", err)
	}

	// If storage id already exists for this sector, update primary if need be
	if currPrimary.Valid {
		if !currPrimary.Bool && primary {
			_, err = dbi.harmonyDB.Exec(ctx,
				"update SectorLocation set is_primary = TRUE where miner_id=$1 and sector_num=$2 and sector_filetype=$3 and storage_id=$4",
				s.Miner, s.Number, ft, storageID)
			if err != nil {
				return xerrors.Errorf("DB update fails: ", err)
			}
		} else {
			log.Warnf("sector %v redeclared in %s", s, storageID)
		}
	} else {
		_, err = dbi.harmonyDB.Exec(ctx,
			"insert into SectorLocation "+
				"values($1, $2, $3, $4, $5)",
			s.Miner, s.Number, ft, storageID, primary)
		if err != nil {
			return xerrors.Errorf("DB insert fails: ", err)
		}
	}

	return nil

}

func (i *Index) StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {
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

func (dbi *DBIndex) StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error {

	ftValid := false
	for _, fileType := range storiface.PathTypes {
		if fileType&ft == 0 {
			ftValid = true
			break
		}
	}
	if !ftValid {
		return xerrors.Errorf("Invalid filetype")
	}

	_, err := dbi.harmonyDB.Exec(ctx,
		"delete from sectorlocation where miner_id=$1 and sector_num=$2 and sector_filetype=$3 and storage_id=$4",
		int(s.Miner), int(s.Number), int(ft), string(storageID))
	if err != nil {
		return xerrors.Errorf("StorageDropSector delete query fails: ", err)
	}

	return nil
}

func (i *Index) StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error {
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

func (dbi *DBIndex) StorageFindSector(ctx context.Context, s abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {

	var result []storiface.SectorStorageInfo

	allowList := make(map[string]struct{})
	storageWithSector := map[string]bool{}

	type dbRes struct {
		Storage_id  string
		Count       uint64
		Is_primary  bool
		Urls        string
		Weight      uint64
		Can_seal    bool
		Can_store   bool
		Groups      string
		Allow_to    string
		Allow_types string
		Deny_types  string
	}

	var rows []dbRes

	fts := ft.AllSet()
	// Find all storage info which already hold this sector + filetype
	err := dbi.harmonyDB.Select(ctx, &rows,
		`		SELECT DISTINCT ON (stor.storage_id)
						  stor.storage_id,
						  COUNT(*) OVER(PARTITION BY stor.storage_id) as count, 
						  BOOL_OR(is_primary) OVER(PARTITION BY stor.storage_id) AS is_primary,
						  urls,
						  weight,
						  can_seal,
						  can_store,
						  groups,
						  allow_to,
						  allow_types,
						  deny_types
						FROM sectorlocation sec
						JOIN storagelocation stor ON sec.storage_id = stor.storage_id 
						WHERE sec.miner_id = $1
						  AND sec.sector_num = $2 
						  AND sec.sector_filetype = ANY($3)  
						ORDER BY stor.storage_id`,
		s.Miner, s.Number, fts)
	if err != nil {
		return nil, xerrors.Errorf("Finding sector storage from DB fails with err: ", err)
	}

	for _, row := range rows {

		// Parse all urls
		var urls, burls []string
		for _, u := range splitString(row.Urls) {
			rl, err := url.Parse(u)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse url: %w", err)
			}
			rl.Path = gopath.Join(rl.Path, ft.String(), storiface.SectorName(s))
			urls = append(urls, rl.String())
			burls = append(burls, u)
		}

		result = append(result, storiface.SectorStorageInfo{
			ID:         storiface.ID(row.Storage_id),
			URLs:       urls,
			BaseURLs:   burls,
			Weight:     row.Weight * row.Count,
			CanSeal:    row.Can_seal,
			CanStore:   row.Can_store,
			Primary:    row.Is_primary,
			AllowTypes: splitString(row.Allow_types),
			DenyTypes:  splitString(row.Deny_types),
		})

		storageWithSector[row.Storage_id] = true

		allowTo := splitString(row.Allow_to)
		if allowList != nil && len(allowTo) > 0 {
			for _, group := range allowTo {
				allowList[group] = struct{}{}
			}
		} else {
			allowList = nil // allow to any
		}
	}

	// Find all storage paths which can hold this sector if allowFetch is true
	if allowFetch {
		spaceReq, err := ft.SealSpaceUse(ssize)
		if err != nil {
			return nil, xerrors.Errorf("estimating required space: %w", err)
		}

		// Conditions to satisfy when choosing a sector
		// 1. CanSeal is true
		// 2. Available >= spaceReq
		// 3. curr_time - last_heartbeat < SkippedHeartbeatThresh
		// 4. heartbeat_err is NULL
		// 5. not one of the earlier picked storage ids
		// 6. !ft.AnyAllowed(st.info.AllowTypes, st.info.DenyTypes)
		// 7. Storage path is part of the groups which are allowed from the storage paths which already hold the sector

		var rows []struct {
			Storage_id  string
			Urls        string
			Weight      uint64
			Can_seal    bool
			Can_store   bool
			Groups      string
			Allow_types string
			Deny_types  string
		}
		err = dbi.harmonyDB.Select(ctx, &rows,
			`select storage_id, 
					  	urls, 
					  	weight,
					  	can_seal,
					  	can_store,
					  	groups, 
					  	allow_types, 
					  	deny_types
				from storagelocation 
				where can_seal=true 
				  and available >= $1 
				  and NOW()-last_heartbeat < $2 
				  and heartbeat_err is null`,
			spaceReq, SkippedHeartbeatThresh)
		if err != nil {
			return nil, xerrors.Errorf("Selecting allowfetch storage paths from DB fails err: ", err)
		}

		for _, row := range rows {
			if ok := storageWithSector[row.Storage_id]; ok {
				continue
			}

			if !ft.AnyAllowed(splitString(row.Allow_types), splitString(row.Deny_types)) {
				log.Debugf("not selecting on %s, not allowed by file type filters", row.Storage_id)
				continue
			}

			if allowList != nil {
				groups := splitString(row.Groups)
				allow := false
				for _, group := range groups {
					if _, found := allowList[group]; found {
						log.Debugf("path %s in allowed group %s", row.Storage_id, group)
						allow = true
						break
					}
				}

				if !allow {
					log.Debugf("not selecting on %s, not in allowed group, allow %+v; path has %+v", row.Storage_id, allowList, groups)
					continue
				}
			}

			var urls, burls []string
			for _, u := range splitString(row.Urls) {
				rl, err := url.Parse(u)
				if err != nil {
					return nil, xerrors.Errorf("failed to parse url: %w", err)
				}
				rl.Path = gopath.Join(rl.Path, ft.String(), storiface.SectorName(s))
				urls = append(urls, rl.String())
				burls = append(burls, u)
			}

			result = append(result, storiface.SectorStorageInfo{
				ID:         storiface.ID(row.Storage_id),
				URLs:       urls,
				BaseURLs:   burls,
				Weight:     row.Weight,
				CanSeal:    row.Can_seal,
				CanStore:   row.Can_store,
				Primary:    false,
				AllowTypes: splitString(row.Allow_types),
				DenyTypes:  splitString(row.Deny_types),
			})
		}
	}

	return result, nil
}

func (i *Index) StorageFindSector(ctx context.Context, s abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {
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

			AllowTypes: st.info.AllowTypes,
			DenyTypes:  st.info.DenyTypes,
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

				AllowTypes: st.info.AllowTypes,
				DenyTypes:  st.info.DenyTypes,
			})
		}
	}

	return out, nil
}

func (dbi *DBIndex) StorageInfo(ctx context.Context, id storiface.ID) (storiface.StorageInfo, error) {

	var qResults []struct {
		Urls        string
		Weight      uint64
		Max_storage uint64
		Can_seal    bool
		Can_store   bool
		Groups      string
		Allow_to    string
		Allow_types string
		Deny_types  string
	}

	err := dbi.harmonyDB.Select(ctx, &qResults,
		"select urls, weight, max_storage, can_seal, can_store, groups, allow_to, allow_types, deny_types "+
			"from StorageLocation where storage_id=$1", string(id))
	if err != nil {
		return storiface.StorageInfo{}, xerrors.Errorf("StorageInfo query fails: ", err)
	}

	var sinfo storiface.StorageInfo
	sinfo.ID = id
	sinfo.URLs = splitString(qResults[0].Urls)
	sinfo.Weight = qResults[0].Weight
	sinfo.MaxStorage = qResults[0].Max_storage
	sinfo.CanSeal = qResults[0].Can_seal
	sinfo.CanStore = qResults[0].Can_store
	sinfo.Groups = splitString(qResults[0].Groups)
	sinfo.AllowTo = splitString(qResults[0].Allow_to)
	sinfo.AllowTypes = splitString(qResults[0].Allow_types)
	sinfo.DenyTypes = splitString(qResults[0].Deny_types)

	return sinfo, nil
}

func (i *Index) StorageInfo(ctx context.Context, id storiface.ID) (storiface.StorageInfo, error) {
	i.lk.RLock()
	defer i.lk.RUnlock()

	si, found := i.stores[id]
	if !found {
		return storiface.StorageInfo{}, xerrors.Errorf("sector store not found")
	}

	return *si.info, nil
}

func (dbi *DBIndex) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]storiface.StorageInfo, error) {
	var err error
	var spaceReq uint64
	switch pathType {
	case storiface.PathSealing:
		spaceReq, err = allocate.SealSpaceUse(ssize)
	case storiface.PathStorage:
		spaceReq, err = allocate.StoreSpaceUse(ssize)
	default:
		return nil, xerrors.Errorf("Unexpected path type")
	}
	if err != nil {
		return nil, xerrors.Errorf("estimating required space: %w", err)
	}

	var checkString string
	if pathType == storiface.PathSealing {
		checkString = "can_seal = TRUE"
	} else if pathType == storiface.PathStorage {
		checkString = "can_store = TRUE"
	}

	var rows []struct {
		Storage_id  string
		Urls        string
		Weight      uint64
		Max_storage uint64
		Can_seal    bool
		Can_store   bool
		Groups      string
		Allow_to    string
		Allow_types string
		Deny_types  string
	}

	sql := fmt.Sprintf(`select storage_id, 
								urls, 
								weight,
								max_storage, 
								can_seal, 
								can_store, 
								groups, 
								allow_to, 
								allow_types, 
								deny_types 
						 from storagelocation 
						 where %s and available >= $1
						 and NOW()-last_heartbeat < $2 
						 and heartbeat_err is null
						order by (available::numeric * weight) desc`, checkString)
	err = dbi.harmonyDB.Select(ctx, &rows,
		sql, spaceReq, SkippedHeartbeatThresh)
	if err != nil {
		return nil, xerrors.Errorf("Querying for best storage sectors fails with sql: %s and err %w: ", sql, err)
	}

	var result []storiface.StorageInfo
	for _, row := range rows {
		result = append(result, storiface.StorageInfo{
			ID:         storiface.ID(row.Storage_id),
			URLs:       splitString(row.Urls),
			Weight:     row.Weight,
			MaxStorage: row.Max_storage,
			CanSeal:    row.Can_seal,
			CanStore:   row.Can_store,
			Groups:     splitString(row.Groups),
			AllowTo:    splitString(row.Allow_to),
			AllowTypes: splitString(row.Allow_types),
			DenyTypes:  splitString(row.Deny_types),
		})
	}

	return result, nil
}

func (i *Index) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType) ([]storiface.StorageInfo, error) {
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

	out := make([]storiface.StorageInfo, len(candidates))
	for i, candidate := range candidates {
		out[i] = *candidate.info
	}

	return out, nil
}

func (i *Index) FindSector(id abi.SectorID, typ storiface.SectorFileType) ([]storiface.ID, error) {
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

func (dbi *DBIndex) StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error {
	// TODO: implementation
	return nil
}

func (dbi *DBIndex) StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error) {
	// TODO: implementation
	return false, nil
}

func (dbi *DBIndex) StorageGetLocks(ctx context.Context) (storiface.SectorLocks, error) {
	// TODO: implementation
	return storiface.SectorLocks{}, nil
}

var _ SectorIndex = &Index{}

var _ SectorIndex = &DBIndex{}

var _ SectorIndex = &IndexProxy{}
