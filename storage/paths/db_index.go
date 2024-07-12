package paths

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	gopath "path"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const NoMinerFilter = abi.ActorID(0)

const URLSeparator = ","

var errAlreadyLocked = errors.New("already locked")

type DBIndex struct {
	alerting   *alerting.Alerting
	pathAlerts map[storiface.ID]alerting.AlertType

	harmonyDB *harmonydb.DB
}

func NewDBIndex(al *alerting.Alerting, db *harmonydb.DB) *DBIndex {
	return &DBIndex{
		harmonyDB: db,

		alerting:   al,
		pathAlerts: map[storiface.ID]alerting.AlertType{},
	}
}

func (dbi *DBIndex) StorageList(ctx context.Context) (map[storiface.ID][]storiface.Decl, error) {

	var sectorEntries []struct {
		StorageId      string
		MinerId        sql.NullInt64
		SectorNum      sql.NullInt64
		SectorFiletype sql.NullInt32 `db:"sector_filetype"`
		IsPrimary      sql.NullBool
	}

	err := dbi.harmonyDB.Select(ctx, &sectorEntries,
		"SELECT stor.storage_id, miner_id, sector_num, sector_filetype, is_primary FROM storage_path stor LEFT JOIN sector_location sec on stor.storage_id=sec.storage_id")
	if err != nil {
		return nil, xerrors.Errorf("StorageList DB query fails: %v", err)
	}

	byID := map[storiface.ID]map[abi.SectorID]storiface.SectorFileType{}
	for _, entry := range sectorEntries {
		id := storiface.ID(entry.StorageId)
		_, ok := byID[id]
		if !ok {
			byID[id] = map[abi.SectorID]storiface.SectorFileType{}
		}

		// skip sector info for storage paths with no sectors
		if !entry.MinerId.Valid {
			continue
		}

		sectorId := abi.SectorID{
			Miner:  abi.ActorID(entry.MinerId.Int64),
			Number: abi.SectorNumber(entry.SectorNum.Int64),
		}

		byID[id][sectorId] |= storiface.SectorFileType(entry.SectorFiletype.Int32)
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

	// Single transaction to attach storage which is not present in the DB
	_, err := dbi.harmonyDB.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
		var urls sql.NullString
		var storageId sql.NullString
		err = tx.QueryRow(
			"SELECT storage_id, urls FROM storage_path WHERE storage_id = $1", string(si.ID)).Scan(&storageId, &urls)
		if err != nil && !strings.Contains(err.Error(), "no rows in result set") {
			return false, xerrors.Errorf("storage attach select fails: %v", err)
		}

		// Storage ID entry exists
		// TODO: Consider using insert into .. on conflict do update set ... below
		if storageId.Valid {
			var currUrls []string
			if urls.Valid {
				currUrls = strings.Split(urls.String, URLSeparator)
			}
			currUrls = union(currUrls, si.URLs)

			_, err = tx.Exec(
				"UPDATE storage_path set urls=$1, weight=$2, max_storage=$3, can_seal=$4, can_store=$5, groups=$6, allow_to=$7, allow_types=$8, deny_types=$9, allow_miners=$10, deny_miners=$11, last_heartbeat=NOW() WHERE storage_id=$12",
				strings.Join(currUrls, URLSeparator),
				si.Weight,
				si.MaxStorage,
				si.CanSeal,
				si.CanStore,
				strings.Join(si.Groups, ","),
				strings.Join(si.AllowTo, ","),
				strings.Join(si.AllowTypes, ","),
				strings.Join(si.DenyTypes, ","),
				strings.Join(si.AllowMiners, ","),
				strings.Join(si.DenyMiners, ","),
				si.ID)
			if err != nil {
				return false, xerrors.Errorf("storage attach UPDATE fails: %v", err)
			}

			return true, nil
		}

		// Insert storage id
		_, err = tx.Exec(
			"INSERT INTO storage_path (storage_id, urls, weight, max_storage, can_seal, can_store, groups, allow_to, allow_types, deny_types, capacity, available, fs_available, reserved, used, last_heartbeat, heartbeat_err, allow_miners, deny_miners)"+
				"Values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NULL, $16, $17)",
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
			strings.Join(si.AllowMiners, ","),
			strings.Join(si.DenyMiners, ","))
		if err != nil {
			return false, xerrors.Errorf("StorageAttach insert fails: %v", err)
		}
		return true, nil
	}, harmonydb.OptionRetry())

	return err
}

func (dbi *DBIndex) StorageDetach(ctx context.Context, id storiface.ID, url string) error {

	// If url not in path urls, error out
	// if this is only path url for this storage path, drop storage path and sector decls which have this as a storage path

	var qUrls string
	err := dbi.harmonyDB.QueryRow(ctx, "SELECT COALESCE(urls,'') FROM storage_path WHERE storage_id=$1", string(id)).Scan(&qUrls)
	if err != nil {
		return err
	}
	urls := splitString(qUrls)

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
		newUrls := strings.Join(modUrls, URLSeparator)
		_, err := dbi.harmonyDB.Exec(ctx, "UPDATE storage_path set urls=$1 WHERE storage_id=$2", newUrls, id)
		if err != nil {
			return err
		}

		log.Warnw("Dropping sector path endpoint", "path", id, "url", url)
	} else {
		// Single transaction to drop storage path and sector decls which have this as a storage path
		_, err := dbi.harmonyDB.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
			// Drop storage path completely
			_, err = tx.Exec("DELETE FROM storage_path WHERE storage_id=$1", id)
			if err != nil {
				return false, err
			}

			// Drop all sectors entries which use this storage path
			_, err = tx.Exec("DELETE FROM sector_location WHERE storage_id=$1", id)
			if err != nil {
				return false, err
			}
			return true, nil
		}, harmonydb.OptionRetry())
		if err != nil {
			return err
		}
		log.Warnw("Dropping sector storage", "path", id)
	}

	return err
}

func (dbi *DBIndex) StorageReportHealth(ctx context.Context, id storiface.ID, report storiface.HealthReport) error {
	retryWait := time.Millisecond * 20
retryReportHealth:
	_, err := dbi.harmonyDB.Exec(ctx,
		"UPDATE storage_path set capacity=$1, available=$2, fs_available=$3, reserved=$4, used=$5, last_heartbeat=NOW() where storage_id=$6",
		report.Stat.Capacity,
		report.Stat.Available,
		report.Stat.FSAvailable,
		report.Stat.Reserved,
		report.Stat.Used,
		id)
	if err != nil {
		//return xerrors.Errorf("updating storage health in DB fails with err: %v", err)
		if harmonydb.IsErrSerialization(err) {
			time.Sleep(retryWait)
			retryWait *= 2
			goto retryReportHealth
		}
		return err
	}

	var canSeal, canStore bool
	err = dbi.harmonyDB.QueryRow(ctx,
		"SELECT can_seal, can_store FROM storage_path WHERE storage_id=$1", id).Scan(&canSeal, &canStore)
	if err != nil {
		return xerrors.Errorf("Querying for storage id %s fails with err %v", id, err)
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

// function to check if a filetype is valid
func (dbi *DBIndex) checkFileType(fileType storiface.SectorFileType) bool {
	ftValid := false
	for _, fileTypeValid := range storiface.PathTypes {
		if fileTypeValid&fileType == 0 {
			ftValid = true
			break
		}
	}
	return ftValid
}

func (dbi *DBIndex) StorageDeclareSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType, primary bool) error {

	if !dbi.checkFileType(ft) {
		return xerrors.Errorf("invalid filetype")
	}

	_, err := dbi.harmonyDB.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
		var currPrimary sql.NullBool
		err = tx.QueryRow(
			"SELECT is_primary FROM sector_location WHERE miner_id=$1 and sector_num=$2 and sector_filetype=$3 and storage_id=$4",
			uint64(s.Miner), uint64(s.Number), int(ft), string(storageID)).Scan(&currPrimary)
		if err != nil && !strings.Contains(err.Error(), "no rows in result set") {
			return false, xerrors.Errorf("DB SELECT fails: %v", err)
		}

		// If storage id already exists for this sector, update primary if need be
		if currPrimary.Valid {
			if !currPrimary.Bool && primary {
				_, err = tx.Exec(
					"UPDATE sector_location set is_primary = TRUE WHERE miner_id=$1 and sector_num=$2 and sector_filetype=$3 and storage_id=$4",
					s.Miner, s.Number, ft, storageID)
				if err != nil {
					return false, xerrors.Errorf("DB update fails: %v", err)
				}
			} else {
				log.Warnf("sector %v redeclared in %s", s, storageID)
			}
		} else {
			_, err = tx.Exec(
				"INSERT INTO sector_location (miner_id, sector_num, sector_filetype, storage_id, is_primary)"+
					"values($1, $2, $3, $4, $5)",
				s.Miner, s.Number, ft, storageID, primary)
			if err != nil {
				return false, xerrors.Errorf("DB insert fails: %v", err)
			}
		}

		return true, nil
	}, harmonydb.OptionRetry())

	return err
}

func (dbi *DBIndex) StorageDropSector(ctx context.Context, storageID storiface.ID, s abi.SectorID, ft storiface.SectorFileType) error {

	if !dbi.checkFileType(ft) {
		return xerrors.Errorf("invalid filetype")
	}

	_, err := dbi.harmonyDB.Exec(ctx,
		"DELETE FROM sector_location WHERE miner_id=$1 and sector_num=$2 and sector_filetype=$3 and storage_id=$4",
		int(s.Miner), int(s.Number), int(ft), string(storageID))
	if err != nil {
		return xerrors.Errorf("StorageDropSector DELETE query fails: %v", err)
	}

	return nil
}

func (dbi *DBIndex) StorageFindSector(ctx context.Context, s abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {

	var result []storiface.SectorStorageInfo

	allowList := make(map[string]struct{})
	storageWithSector := map[string]bool{}

	type dbRes struct {
		StorageId  string
		Count      uint64
		IsPrimary  bool
		Urls       string
		Weight     uint64
		CanSeal    bool
		CanStore   bool
		Groups     string
		AllowTo    string
		AllowTypes string
		DenyTypes  string
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
						FROM sector_location sec
						JOIN storage_path stor ON sec.storage_id = stor.storage_id 
						WHERE sec.miner_id = $1
						  AND sec.sector_num = $2 
						  AND sec.sector_filetype = ANY($3)  
						ORDER BY stor.storage_id`,
		s.Miner, s.Number, fts)
	if err != nil {
		return nil, xerrors.Errorf("Finding sector storage from DB fails with err: %v", err)
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
			ID:         storiface.ID(row.StorageId),
			URLs:       urls,
			BaseURLs:   burls,
			Weight:     row.Weight * row.Count,
			CanSeal:    row.CanSeal,
			CanStore:   row.CanStore,
			Primary:    row.IsPrimary,
			AllowTypes: splitString(row.AllowTypes),
			DenyTypes:  splitString(row.DenyTypes),
		})

		storageWithSector[row.StorageId] = true

		allowTo := splitString(row.AllowTo)
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
			StorageId   string
			Urls        string
			Weight      uint64
			CanSeal     bool
			CanStore    bool
			Groups      string
			AllowTypes  string
			DenyTypes   string
			AllowMiners string
			DenyMiners  string
		}
		err = dbi.harmonyDB.Select(ctx, &rows,
			`SELECT storage_id, 
					  	urls, 
					  	weight,
					  	can_seal,
					  	can_store,
					  	groups, 
					  	allow_types, 
					  	deny_types,
					  	allow_miners,
					  	deny_miners
				FROM storage_path 
				WHERE can_seal=true 
				  and available >= $1 
				  and NOW()-($2 * INTERVAL '1 second') < last_heartbeat
				  and heartbeat_err is null`,
			spaceReq, SkippedHeartbeatThresh.Seconds())
		if err != nil {
			return nil, xerrors.Errorf("Selecting allowfetch storage paths from DB fails err: %v", err)
		}

		for _, row := range rows {
			if ok := storageWithSector[row.StorageId]; ok {
				continue
			}

			if !ft.AnyAllowed(splitString(row.AllowTypes), splitString(row.DenyTypes)) {
				log.Debugf("not selecting on %s, not allowed by file type filters", row.StorageId)
				continue
			}
			allowMiners := splitString(row.AllowMiners)
			denyMiners := splitString(row.DenyMiners)
			proceed, msg, err := MinerFilter(allowMiners, denyMiners, s.Miner)
			if err != nil {
				return nil, err
			}
			if !proceed {
				log.Debugf("not allocating on %s, miner %s %s", row.StorageId, s.Miner.String(), msg)
				continue
			}

			if allowList != nil {
				groups := splitString(row.Groups)
				allow := false
				for _, group := range groups {
					if _, found := allowList[group]; found {
						log.Debugf("path %s in allowed group %s", row.StorageId, group)
						allow = true
						break
					}
				}

				if !allow {
					log.Debugf("not selecting on %s, not in allowed group, allow %+v; path has %+v", row.StorageId, allowList, groups)
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
				ID:         storiface.ID(row.StorageId),
				URLs:       urls,
				BaseURLs:   burls,
				Weight:     row.Weight * 0,
				CanSeal:    row.CanSeal,
				CanStore:   row.CanStore,
				Primary:    false,
				AllowTypes: splitString(row.AllowTypes),
				DenyTypes:  splitString(row.DenyTypes),
			})
		}
	}

	return result, nil
}

func (dbi *DBIndex) StorageInfo(ctx context.Context, id storiface.ID) (storiface.StorageInfo, error) {

	var qResults []struct {
		Urls        string
		Weight      uint64
		MaxStorage  uint64
		CanSeal     bool
		CanStore    bool
		Groups      string
		AllowTo     string
		AllowTypes  string
		DenyTypes   string
		AllowMiners string
		DenyMiners  string
	}

	err := dbi.harmonyDB.Select(ctx, &qResults,
		"SELECT urls, weight, max_storage, can_seal, can_store, groups, allow_to, allow_types, deny_types, allow_miners, deny_miners "+
			"FROM storage_path WHERE storage_id=$1", string(id))
	if err != nil {
		return storiface.StorageInfo{}, xerrors.Errorf("StorageInfo query fails: %v", err)
	}

	var sinfo storiface.StorageInfo
	sinfo.ID = id
	sinfo.URLs = splitString(qResults[0].Urls)
	sinfo.Weight = qResults[0].Weight
	sinfo.MaxStorage = qResults[0].MaxStorage
	sinfo.CanSeal = qResults[0].CanSeal
	sinfo.CanStore = qResults[0].CanStore
	sinfo.Groups = splitString(qResults[0].Groups)
	sinfo.AllowTo = splitString(qResults[0].AllowTo)
	sinfo.AllowTypes = splitString(qResults[0].AllowTypes)
	sinfo.DenyTypes = splitString(qResults[0].DenyTypes)
	sinfo.AllowMiners = splitString(qResults[0].AllowMiners)
	sinfo.DenyMiners = splitString(qResults[0].DenyMiners)

	return sinfo, nil
}

func (dbi *DBIndex) StorageBestAlloc(ctx context.Context, allocate storiface.SectorFileType, ssize abi.SectorSize, pathType storiface.PathType, miner abi.ActorID) ([]storiface.StorageInfo, error) {
	var err error
	var spaceReq uint64
	switch pathType {
	case storiface.PathSealing:
		spaceReq, err = allocate.SealSpaceUse(ssize)
	case storiface.PathStorage:
		spaceReq, err = allocate.StoreSpaceUse(ssize)
	default:
		return nil, xerrors.Errorf("unexpected path type")
	}
	if err != nil {
		return nil, xerrors.Errorf("estimating required space: %w", err)
	}

	var rows []struct {
		StorageId   string
		Urls        string
		Weight      uint64
		MaxStorage  uint64
		CanSeal     bool
		CanStore    bool
		Groups      string
		AllowTo     string
		AllowTypes  string
		DenyTypes   string
		AllowMiners string
		DenyMiners  string
	}

	err = dbi.harmonyDB.Select(ctx, &rows,
		`SELECT storage_id, 
								urls, 
								weight,
								max_storage, 
								can_seal, 
								can_store, 
								groups, 
								allow_to, 
								allow_types, 
								deny_types,
								allow_miners,
								deny_miners
						 FROM storage_path 
						 WHERE available >= $1
						 and NOW()-($2 * INTERVAL '1 second') < last_heartbeat
						 and heartbeat_err IS NULL
						 and (($3 and can_seal = TRUE) or ($4 and can_store = TRUE))
						order by (available::numeric * weight) desc`,
		spaceReq,
		SkippedHeartbeatThresh.Seconds(),
		pathType == storiface.PathSealing,
		pathType == storiface.PathStorage,
	)
	if err != nil {
		return nil, xerrors.Errorf("Querying for best storage sectors fails with err %w: ", err)
	}

	var result []storiface.StorageInfo

	for _, row := range rows {
		// Matching with 0 as a workaround to avoid having minerID
		// present when calling TaskStorage.HasCapacity()
		if miner != NoMinerFilter {
			allowMiners := splitString(row.AllowMiners)
			denyMiners := splitString(row.DenyMiners)
			proceed, msg, err := MinerFilter(allowMiners, denyMiners, miner)
			if err != nil {
				return nil, err
			}
			if !proceed {
				log.Debugf("not allocating on %s, miner %s %s", row.StorageId, miner.String(), msg)
				continue
			}
		}

		result = append(result, storiface.StorageInfo{
			ID:          storiface.ID(row.StorageId),
			URLs:        splitString(row.Urls),
			Weight:      row.Weight,
			MaxStorage:  row.MaxStorage,
			CanSeal:     row.CanSeal,
			CanStore:    row.CanStore,
			Groups:      splitString(row.Groups),
			AllowTo:     splitString(row.AllowTo),
			AllowTypes:  splitString(row.AllowTypes),
			DenyTypes:   splitString(row.DenyTypes),
			AllowMiners: splitString(row.AllowMiners),
			DenyMiners:  splitString(row.DenyMiners),
		})
	}

	return result, nil
}

// timeout after which we consider a lock to be stale
const LockTimeOut = 300 * time.Second

func isLocked(ts sql.NullTime) bool {
	return ts.Valid && ts.Time.After(time.Now().Add(-LockTimeOut))
}

func (dbi *DBIndex) lock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType, lockUuid uuid.UUID) (bool, error) {
	if read|write == 0 {
		return false, nil
	}

	if read|write > (1<<storiface.FileTypes)-1 {
		return false, xerrors.Errorf("unknown file types specified")
	}

	var rows []struct {
		SectorFileType storiface.SectorFileType `db:"sector_filetype"`
		ReadTs         sql.NullTime
		ReadRefs       int
		WriteTs        sql.NullTime
	}
	_, err := dbi.harmonyDB.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {

		fts := (read | write).AllSet()
		err = tx.Select(&rows,
			`SELECT sector_filetype, read_ts, read_refs, write_ts 
								 FROM sector_location 
								 WHERE miner_id=$1 
								   AND sector_num=$2
								   AND sector_filetype = ANY($3)`,
			sector.Miner, sector.Number, fts)
		if err != nil {
			return false, xerrors.Errorf("StorageLock SELECT fails: %v", err)
		}

		type locks struct {
			readTs   sql.NullTime
			readRefs int
			writeTs  sql.NullTime
		}
		lockMap := make(map[storiface.SectorFileType]locks)
		for _, row := range rows {
			lockMap[row.SectorFileType] = locks{
				readTs:   row.ReadTs,
				readRefs: row.ReadRefs,
				writeTs:  row.WriteTs,
			}
		}

		// Check if we can acquire write locks
		// Conditions: No write lock or write lock is stale, No read lock or read lock is stale
		for _, wft := range write.AllSet() {
			if isLocked(lockMap[wft].writeTs) || isLocked(lockMap[wft].readTs) {
				return false, xerrors.Errorf("cannot acquire writelock for sector %v filetype %d already locked: %w", sector, wft, errAlreadyLocked)
			}
		}

		// Check if we can acquire read locks
		// Conditions: No write lock or write lock is stale
		for _, rft := range read.AllSet() {
			if isLocked(lockMap[rft].writeTs) {
				return false, xerrors.Errorf("cannot acquire read lock for sector %v filetype %d already locked for writing: %w", sector, rft, errAlreadyLocked)
			}
		}

		// Acquire write locks
		_, err = tx.Exec(
			`UPDATE sector_location
							 	SET write_ts = NOW(), write_lock_owner = $1
							 	WHERE miner_id=$2
							 		AND sector_num=$3
							 		AND sector_filetype = ANY($4)`,
			lockUuid.String(),
			sector.Miner,
			sector.Number,
			write.AllSet())
		if err != nil {
			return false, xerrors.Errorf("acquiring write locks for sector %v fails with err: %v", sector, err)
		}

		// Acquire read locks
		_, err = tx.Exec(
			`UPDATE sector_location
							 	SET read_ts = NOW(), read_refs = read_refs + 1
							 	WHERE miner_id=$1
							 		AND sector_num=$2
							 		AND sector_filetype = ANY($3)`,
			sector.Miner,
			sector.Number,
			read.AllSet())
		if err != nil {
			return false, xerrors.Errorf("acquiring read locks for sector %v fails with err: %v", sector, err)
		}

		return true, nil
	}, harmonydb.OptionRetry())
	if err != nil {
		return false, err
	}

	return true, nil
}

func (dbi *DBIndex) unlock(sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType, lockUuid uuid.UUID) (bool, error) {
	ctx := context.Background()

	if read|write == 0 {
		return false, nil
	}

	if read|write > (1<<storiface.FileTypes)-1 {
		return false, xerrors.Errorf("unknown file types specified")
	}

	// Relinquish write locks
	_, err := dbi.harmonyDB.Exec(ctx,
		`UPDATE sector_location
				SET write_ts = NULL, write_lock_owner = NULL
				WHERE miner_id=$1
					AND sector_num=$2
					AND write_lock_owner=$3
					AND sector_filetype = ANY($4)`,
		sector.Miner,
		sector.Number,
		lockUuid.String(),
		write.AllSet())
	if err != nil {
		return false, xerrors.Errorf("relinquishing write locks for sector %v fails with err: %v", sector, err)
	}

	// Relinquish read locks
	_, err = dbi.harmonyDB.Exec(ctx,
		`UPDATE sector_location
				SET read_refs = read_refs-1,
					read_ts = CASE WHEN read_refs - 1 = 0 THEN NULL ELSE read_ts END
				WHERE miner_id=$1
					AND sector_num=$2
					AND sector_filetype = ANY($3)`,
		sector.Miner,
		sector.Number,
		read.AllSet())
	if err != nil {
		return false, xerrors.Errorf("relinquishing read locks for sector %v fails with err: %v", sector, err)
	}

	return true, nil

}

func (dbi *DBIndex) StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error {

	retries := 5
	maxWaitTime := 300 // Set max wait time to 5 minutes

	waitTime := 1
	// generate uuid for this lock owner
	lockUuid := uuid.New()

	// retry with exponential backoff and block until lock is acquired
	for {
		locked, err := dbi.lock(ctx, sector, read, write, lockUuid)
		// if err is not nil and is not because we cannot acquire lock, retry
		if err != nil && !errors.Is(err, errAlreadyLocked) {
			retries--
			if retries == 0 {
				return err
			}
		}

		if locked {
			break
		}

		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
			if waitTime < maxWaitTime {
				waitTime *= 2
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	go func() {
		<-ctx.Done()
		_, err := dbi.unlock(sector, read, write, lockUuid)
		if err != nil {
			log.Errorf("unlocking sector %v for filetypes: read=%d, write=%d, fails with err: %v", sector, read, write, err)
		}

	}()

	return nil
}

func (dbi *DBIndex) StorageTryLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) (bool, error) {
	lockUuid := uuid.New()
	locked, err := dbi.lock(ctx, sector, read, write, lockUuid)
	if err != nil {
		return false, err
	}

	if locked {
		go func() {
			<-ctx.Done()
			_, err := dbi.unlock(sector, read, write, lockUuid)
			if err != nil {
				log.Errorf("unlocking sector %v for filetypes: read=%d, write=%d, fails with err: %v", sector, read, write, err)
			}
		}()
	}

	return locked, nil
}

func (dbi *DBIndex) StorageGetLocks(ctx context.Context) (storiface.SectorLocks, error) {

	var rows []struct {
		MinerId        uint64
		SectorNum      uint64
		SectorFileType int `db:"sector_filetype"`
		ReadTs         sql.NullTime
		ReadRefs       int
		WriteTs        sql.NullTime
	}

	err := dbi.harmonyDB.Select(ctx, &rows,
		"SELECT miner_id, sector_num, sector_filetype, read_ts, read_refs, write_ts FROM sector_location")
	if err != nil {
		return storiface.SectorLocks{}, err
	}

	type locks struct {
		sectorFileType storiface.SectorFileType
		readRefs       uint
		writeLk        bool
	}
	sectorLocks := make(map[abi.SectorID]locks)
	for _, row := range rows {
		sector := abi.SectorID{
			Miner:  abi.ActorID(row.MinerId),
			Number: abi.SectorNumber(row.SectorNum),
		}

		var readRefs uint
		if isLocked(row.ReadTs) {
			readRefs = uint(row.ReadRefs)
		}

		sectorLocks[sector] = locks{
			sectorFileType: storiface.SectorFileType(row.SectorFileType),
			readRefs:       readRefs,
			writeLk:        isLocked(row.WriteTs),
		}
	}

	var result storiface.SectorLocks
	for sector, locks := range sectorLocks {
		var lock storiface.SectorLock
		lock.Sector = sector
		lock.Read[locks.sectorFileType] = locks.readRefs
		if locks.writeLk {
			lock.Write[locks.sectorFileType] = 1
		} else {
			lock.Write[locks.sectorFileType] = 0
		}
		result.Locks = append(result.Locks, lock)
	}

	return result, nil
}

var _ SectorIndex = &DBIndex{}
