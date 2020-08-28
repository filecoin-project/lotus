package scheduler

import (
	"context"
	"database/sql"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"golang.org/x/xerrors"
)

var log = logging.Logger("scheduler")

// Scheduler manages the execution of jobs triggered
// by tickers. Not externally configurable at runtime.
type Scheduler struct {
	db *sql.DB
}

// PrepareScheduler returns a ready-to-run Scheduler
func PrepareScheduler(db *sql.DB) *Scheduler {
	return &Scheduler{db}
}

func (s *Scheduler) setupSchema(ctx context.Context) error {
	if err := setupTopMinerByBaseRewardSchema(ctx, s.db); err != nil {
		return xerrors.Errorf("setup top miners by reward schema: %w", err)
	}
	return nil
}

// Start the scheduler jobs at the defined intervals
func (s *Scheduler) Start(ctx context.Context) {
	log.Debug("Starting Scheduler")

	if err := s.setupSchema(ctx); err != nil {
		log.Fatalw("applying scheduling schema", "error", err)
	}

	go func() {
		// run once on start after schema has initialized
		time.Sleep(1 * time.Minute)
		if err := refreshTopMinerByBaseReward(ctx, s.db); err != nil {
			log.Errorw("failed to refresh top miner", "error", err)
		}
		refreshTopMinerCh := time.NewTicker(30 * time.Second)
		defer refreshTopMinerCh.Stop()
		for {
			select {
			case <-refreshTopMinerCh.C:
				if err := refreshTopMinerByBaseReward(ctx, s.db); err != nil {
					log.Errorw("failed to refresh top miner", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
