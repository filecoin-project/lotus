package scheduler

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/xerrors"
)

func refreshTopMinerByBaseReward(ctx context.Context, db *sql.DB) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	t := time.Now()
	defer func() {
		log.Debugw("refresh top_miners_by_base_reward", "duration", time.Since(t).String())
	}()

	_, err := db.Exec("REFRESH MATERIALIZED VIEW top_miners_by_base_reward;")
	if err != nil {
		return xerrors.Errorf("refresh top_miners_by_base_reward: %w", err)
	}

	return nil
}
