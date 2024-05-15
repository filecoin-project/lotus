package gc

import (
	"context"
	"strings"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("curiogc")

const StorageEndpointGCInterval = 21 * time.Minute
const StorageEndpointDeadTime = StorageEndpointGCInterval * 6 // ~2h
const MaxParallelEndpointChecks = 32

type StorageEndpointGC struct {
	si     *paths.DBIndex
	remote *paths.Remote
	db     *harmonydb.DB
}

func NewStorageEndpointGC(si *paths.DBIndex, remote *paths.Remote, db *harmonydb.DB) *StorageEndpointGC {
	return &StorageEndpointGC{
		si:     si,
		remote: remote,
		db:     db,
	}
}

func (s *StorageEndpointGC) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	/*
		1. Get all storage paths + urls (endpoints)
		2. Ping each url, record results
		3. Update sector_path_url_liveness with success/failure
		4.1 If a URL was consistently down for StorageEndpointDeadTime, remove it from the storage_path table
		4.2 Remove storage paths with no URLs remaining
		4.2.1 in the same transaction remove sector refs to the dead path
	*/

	ctx := context.Background()

	var pathRefs []struct {
		StorageID     storiface.ID `db:"storage_id"`
		Urls          string       `db:"urls"`
		LastHeartbeat *time.Time   `db:"last_heartbeat"`
	}

	err = s.db.Select(ctx, &pathRefs, `SELECT storage_id, urls, last_heartbeat FROM storage_path`)
	if err != nil {
		return false, xerrors.Errorf("getting path metadata: %w", err)
	}

	type pingResult struct {
		storageID storiface.ID
		url       string

		res result.Result[fsutil.FsStat]
	}

	var pingResults []pingResult
	var resultLk sync.Mutex
	var resultThrottle = make(chan struct{}, MaxParallelEndpointChecks)

	for _, pathRef := range pathRefs {
		pathRef := pathRef
		urls := strings.Split(pathRef.Urls, paths.URLSeparator)

		for _, url := range urls {
			url := url

			select {
			case resultThrottle <- struct{}{}:
			case <-ctx.Done():
				return false, ctx.Err()
			}

			go func() {
				defer func() {
					<-resultThrottle
				}()

				st, err := s.remote.StatUrl(ctx, url, pathRef.StorageID)

				res := pingResult{
					storageID: pathRef.StorageID,
					url:       url,
					res:       result.Wrap(st, err),
				}

				resultLk.Lock()
				pingResults = append(pingResults, res)
				resultLk.Unlock()
			}()
		}
	}

	// Wait for all pings to finish
	for i := 0; i < MaxParallelEndpointChecks; i++ {
		select {
		case resultThrottle <- struct{}{}:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	// Update the liveness table

	/*
		create table sector_path_url_liveness (
		    storage_id text,
		    url text,

		    last_checked timestamp not null,
		    last_live timestamp,
		    last_dead timestamp,
		    last_dead_reason text,

		    primary key (storage_id, url),

		    foreign key (storage_id) references storage_path (storage_id) on delete cascade
		)
	*/

	currentTime := time.Now().UTC()

	committed, err := s.db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (bool, error) {
		for _, pingResult := range pingResults {
			var lastLive, lastDead, lastDeadReason interface{}
			if pingResult.res.Error == nil {
				lastLive = currentTime.UTC()
				lastDead = nil
				lastDeadReason = nil
			} else {
				lastLive = nil
				lastDead = currentTime.UTC()
				lastDeadReason = pingResult.res.Error.Error()
			}

			// This function updates the liveness data for a URL in the `sector_path_url_liveness` table.
			//
			// On conflict, where the same `storage_id` and `url` are found:
			// - last_checked is always updated to the current timestamp.
			// - last_live is updated to the new `last_live` if it is not null; otherwise, it retains the existing value.
			// - last_dead is conditionally updated based on two criteria:
			//   1. It is set to the new `last_dead` if the existing `last_dead` is null (indicating this is the first recorded failure).
			//   2. It is updated to the new `last_dead` if there has been a live instance recorded after the most recent dead timestamp, indicating the resource was alive again before this new failure.
			//   3. It retains the existing value if none of the above conditions are met.
			// - last_dead_reason is updated similarly to `last_live`, using COALESCE to prefer the new reason if it's provided.
			_, err := tx.Exec(`
					INSERT INTO sector_path_url_liveness (storage_id, url, last_checked, last_live, last_dead, last_dead_reason)
					VALUES ($1, $2, $3, $4, $5, $6)
					ON CONFLICT (storage_id, url) DO UPDATE
					SET last_checked = EXCLUDED.last_checked,
						last_live = COALESCE(EXCLUDED.last_live, sector_path_url_liveness.last_live),
						last_dead = CASE
							WHEN sector_path_url_liveness.last_dead IS NULL THEN EXCLUDED.last_dead
							WHEN sector_path_url_liveness.last_dead IS NOT NULL AND sector_path_url_liveness.last_live > sector_path_url_liveness.last_dead THEN EXCLUDED.last_dead
							ELSE sector_path_url_liveness.last_dead
						END,
						last_dead_reason = COALESCE(EXCLUDED.last_dead_reason, sector_path_url_liveness.last_dead_reason)
				`, pingResult.storageID, pingResult.url, currentTime, lastLive, lastDead, lastDeadReason)
			if err != nil {
				return false, xerrors.Errorf("updating liveness data: %w", err)
			}
		}

		return true, nil
	}, harmonydb.OptionRetry())
	if err != nil {
		return false, xerrors.Errorf("sector_path_url_liveness update: %w", err)
	}
	if !committed {
		return false, xerrors.Errorf("sector_path_url_liveness update: transaction didn't commit")
	}

	///////
	// Now we do the actual database cleanup
	if !stillOwned() {
		return false, xerrors.Errorf("task no longer owned")
	}

	committed, err = s.db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (bool, error) {
		// Identify URLs that are consistently down
		var deadURLs []struct {
			StorageID storiface.ID
			URL       string
		}
		err = tx.Select(&deadURLs, `
			SELECT storage_id, url FROM sector_path_url_liveness
			WHERE last_dead > COALESCE(last_live, '1970-01-01') AND last_dead < $1
		`, currentTime.Add(-StorageEndpointDeadTime).UTC())
		if err != nil {
			return false, xerrors.Errorf("selecting dead URLs: %w", err)
		}

		log.Debugw("dead urls", "dead_urls", deadURLs)

		// Remove dead URLs from storage_path entries and handle path cleanup
		for _, du := range deadURLs {
			du := du
			// Fetch the current URLs for the storage path
			var URLs string
			err = tx.QueryRow("SELECT urls FROM storage_path WHERE storage_id = $1", du.StorageID).Scan(&URLs)
			if err != nil {
				return false, xerrors.Errorf("fetching storage paths: %w", err)
			}

			// Filter out the dead URL using lo.Reject and prepare the updated list
			urls := strings.Split(URLs, paths.URLSeparator)
			urls = lo.Reject(urls, func(u string, _ int) bool {
				return u == du.URL
			})

			log.Debugw("filtered urls", "urls", urls, "dead_url", du.URL, "storage_id", du.StorageID)

			if len(urls) == 0 {
				// If no URLs left, remove the storage path entirely
				_, err = tx.Exec("DELETE FROM storage_path WHERE storage_id = $1", du.StorageID)
				if err != nil {
					return false, xerrors.Errorf("deleting storage path: %w", err)
				}
				_, err = tx.Exec("DELETE FROM sector_location WHERE storage_id = $1", du.StorageID)
				if err != nil {
					return false, xerrors.Errorf("deleting sector locations: %w", err)
				}
			} else {
				// Update the storage path with the filtered URLs
				newURLs := strings.Join(urls, paths.URLSeparator)
				_, err = tx.Exec("UPDATE storage_path SET urls = $1 WHERE storage_id = $2", newURLs, du.StorageID)
				if err != nil {
					return false, xerrors.Errorf("updating storage path urls: %w", err)
				}
				// Remove sector_path_url_liveness entry
				_, err = tx.Exec("DELETE FROM sector_path_url_liveness WHERE storage_id = $1 AND url = $2", du.StorageID, du.URL)
				if err != nil {
					return false, xerrors.Errorf("deleting sector_path_url_liveness entry: %w", err)
				}
			}
		}

		return true, nil
	}, harmonydb.OptionRetry())
	if err != nil {
		return false, xerrors.Errorf("removing dead URLs and cleaning storage paths: %w", err)
	}
	if !committed {
		return false, xerrors.Errorf("transaction for removing dead URLs and cleaning paths did not commit")
	}

	return true, nil
}

func (s *StorageEndpointGC) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	id := ids[0]
	return &id, nil
}

func (s *StorageEndpointGC) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  1,
		Name: "StorageMetaGC",
		Cost: resources.Resources{
			Cpu: 1,
			Ram: 64 << 20,
			Gpu: 0,
		},
		IAmBored: harmonytask.SingletonTaskAdder(StorageEndpointGCInterval, s),
	}
}

func (s *StorageEndpointGC) Adder(taskFunc harmonytask.AddTaskFunc) {
	// lazy endpoint, added when bored
	return
}

var _ harmonytask.TaskInterface = &StorageEndpointGC{}
