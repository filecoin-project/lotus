package scheduler

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/xerrors"
)

func setupTopMinerByBaseRewardSchema(ctx context.Context, db *sql.DB) error {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		create materialized view if not exists top_miners_by_base_reward as
			with total_rewards_by_miner as (
				select
					b.miner,
					sum(bbr.base_block_reward * b.win_count) as total_reward
				from blocks b
				inner join base_block_rewards bbr on b.parentstateroot = bbr.state_root
				group by 1
			) select
				rank() over (order by total_reward desc),
				miner,
				total_reward
			from total_rewards_by_miner
			group by 2, 3;

		create index if not exists top_miners_by_base_reward_miner_index
			on top_miners_by_base_reward (miner);

		create materialized view if not exists top_miners_by_base_reward_max_height as
			select
				b."timestamp"as current_timestamp,
				max(b.height) as current_height
			from blocks b
			join base_block_rewards bbr on b.parentstateroot = bbr.state_root
			where bbr.base_block_reward is not null
			group by 1
			order by 1 desc
			limit 1;
	`); err != nil {
		return xerrors.Errorf("create top_miner_by_base_reward views: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("committing top_miner_by_base_reward views; %w", err)
	}
	return nil
}

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

	_, err := db.Exec("refresh materialized view top_miners_by_base_reward;")
	if err != nil {
		return xerrors.Errorf("refresh top_miners_by_base_reward: %w", err)
	}

	_, err = db.Exec("refresh materialized view top_miners_by_base_reward_max_height;")
	if err != nil {
		return xerrors.Errorf("refresh top_miners_by_base_reward_max_height: %w", err)
	}

	return nil
}
