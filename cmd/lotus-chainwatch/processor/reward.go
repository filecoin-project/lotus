package processor

import (
	"bytes"
	"context"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
)

type rewardActorInfo struct {
	common actorInfo

	// expected power in bytes during this epoch
	baselinePower big.Int

	// base reward in attofil for each block found during this epoch
	baseBlockReward big.Int
}

func (p *Processor) setupRewards() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
/*
* captures base block reward per miner per state root and does not
* include penalties or gas reward
*/
create table if not exists base_block_rewards
(
	state_root text not null
		constraint block_rewards_pk
			primary key,
	base_block_reward numeric not null
);

/* captures chain-specific power state for any given stateroot */
create table if not exists chain_power
(
	state_root text not null
		constraint chain_power_pk
			primary key,
	baseline_power text not null
);

create materialized view if not exists top_miners_by_base_reward as
	with total_rewards_by_miner as (
		select
			b.miner,
			sum(bbr.base_block_reward) as total_reward
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
`); err != nil {
		return err
	}

	return tx.Commit()

}

func (p *Processor) HandleRewardChanges(ctx context.Context, rewardTips ActorTips) error {
	rewardChanges, err := p.processRewardActors(ctx, rewardTips)
	if err != nil {
		log.Fatalw("Failed to process reward actors", "error", err)
	}

	if err := p.persistRewardActors(ctx, rewardChanges); err != nil {
		return err
	}

	return nil
}

func (p *Processor) processRewardActors(ctx context.Context, rewardTips ActorTips) ([]rewardActorInfo, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Processed Reward Actors", "duration", time.Since(start).String())
	}()

	var out []rewardActorInfo
	for tipset, rewards := range rewardTips {
		for _, act := range rewards {
			var rw rewardActorInfo
			rw.common = act

			// get reward actor states at each tipset once for all updates
			rewardActor, err := p.node.StateGetActor(ctx, builtin.RewardActorAddr, tipset)
			if err != nil {
				return nil, xerrors.Errorf("get reward state (@ %s): %w", rw.common.stateroot.String(), err)
			}

			rewardStateRaw, err := p.node.ChainReadObj(ctx, rewardActor.Head)
			if err != nil {
				return nil, xerrors.Errorf("read state obj (@ %s): %w", rw.common.stateroot.String(), err)
			}

			var rewardActorState reward.State
			if err := rewardActorState.UnmarshalCBOR(bytes.NewReader(rewardStateRaw)); err != nil {
				return nil, xerrors.Errorf("unmarshal state (@ %s): %w", rw.common.stateroot.String(), err)
			}

			rw.baseBlockReward = rewardActorState.LastPerEpochReward
			rw.baselinePower = rewardActorState.BaselinePower
			out = append(out, rw)
		}
	}
	return out, nil
}

func (p *Processor) persistRewardActors(ctx context.Context, rewards []rewardActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Persisted Reward Actors", "duration", time.Since(start).String())
	}()

	grp, ctx := errgroup.WithContext(ctx)

	grp.Go(func() error {
		if err := p.storeChainPower(rewards); err != nil {
			return err
		}
		return nil
	})

	grp.Go(func() error {
		if err := p.storeBaseBlockReward(rewards); err != nil {
			return err
		}
		return nil
	})

	return grp.Wait()
}

func (p *Processor) storeChainPower(rewards []rewardActorInfo) error {
	tx, err := p.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin chain_power tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table cp (like chain_power excluding constraints) on commit drop`); err != nil {
		return xerrors.Errorf("prep chain_power temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy cp (state_root, baseline_power) from STDIN`)
	if err != nil {
		return xerrors.Errorf("prepare tmp chain_power: %w", err)
	}

	for _, rewardState := range rewards {
		if _, err := stmt.Exec(
			rewardState.common.stateroot.String(),
			rewardState.baselinePower.String(),
		); err != nil {
			log.Errorw("failed to store chain power", "state_root", rewardState.common.stateroot, "error", err)
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("close prepared chain_power: %w", err)
	}

	if _, err := tx.Exec(`insert into chain_power select * from cp on conflict do nothing`); err != nil {
		return xerrors.Errorf("insert chain_power from tmp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit chain_power tx: %w", err)
	}

	return nil
}

func (p *Processor) storeBaseBlockReward(rewards []rewardActorInfo) error {
	tx, err := p.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin base_block_reward tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table bbr (like base_block_rewards excluding constraints) on commit drop`); err != nil {
		return xerrors.Errorf("prep base_block_reward temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy bbr (state_root, base_block_reward) from STDIN`)
	if err != nil {
		return xerrors.Errorf("prepare tmp base_block_reward: %w", err)
	}

	for _, rewardState := range rewards {
		baseBlockReward := big.Div(rewardState.baseBlockReward, big.NewIntUnsigned(build.BlocksPerEpoch))
		if _, err := stmt.Exec(
			rewardState.common.stateroot.String(),
			baseBlockReward.String(),
		); err != nil {
			log.Errorw("failed to store base block reward", "state_root", rewardState.common.stateroot, "error", err)
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("close prepared base_block_reward: %w", err)
	}

	if _, err := tx.Exec(`insert into base_block_rewards select * from bbr on conflict do nothing`); err != nil {
		return xerrors.Errorf("insert base_block_reward from tmp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit base_block_reward tx: %w", err)
	}

	return nil
}
