package scheduler

import (
	"context"
	"database/sql"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("scheduler")

// Scheduler manages the execution of jobs triggered
// by tickers. Not externally configuable at runtime.
type Scheduler struct {
	db *sql.DB
}

// PrepareScheduler returns a ready-to-run Scheduler
func PrepareScheduler(db *sql.DB) *Scheduler {
	return &Scheduler{db}
}

// Start the scheduler jobs at the defined intervals
func (s *Scheduler) Start(ctx context.Context) {
	log.Debug("Starting Scheduler")

	go func() {
		// run once on start after schema has initialized
		time.Sleep(5 * time.Second)
		if err := refreshTopMinerByBaseReward(ctx, s.db); err != nil {
			log.Errorf(err.Error())
		}
		refreshTopMinerCh := time.NewTicker(6 * time.Hour)
		defer refreshTopMinerCh.Stop()
		for {
			select {
			case <-refreshTopMinerCh.C:
				if err := refreshTopMinerByBaseReward(ctx, s.db); err != nil {
					log.Errorf(err.Error())
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
