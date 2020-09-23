package processor

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"

	cw_util "github.com/filecoin-project/lotus/cmd/lotus-chainwatch/util"
)

type rewardActorInfo struct {
	common actorInfo

	cumSumBaselinePower big.Int
	cumSumRealizedPower big.Int

	effectiveNetworkTime   abi.ChainEpoch
	effectiveBaselinePower big.Int

	// NOTE: These variables are wrong. Talk to @ZX about fixing. These _do
	// not_ represent "new" anything.
	newBaselinePower     big.Int
	newBaseReward        big.Int
	newSmoothingEstimate builtin.FilterEstimate

	totalMinedReward big.Int
}

func (rw *rewardActorInfo) set(s reward.State) (err error) {
	rw.cumSumBaselinePower, err = s.CumsumBaseline()
	if err != nil {
		return xerrors.Errorf("getting cumsum baseline power (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.cumSumRealizedPower, err = s.CumsumRealized()
	if err != nil {
		return xerrors.Errorf("getting cumsum realized power (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.effectiveNetworkTime, err = s.EffectiveNetworkTime()
	if err != nil {
		return xerrors.Errorf("getting effective network time (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.effectiveBaselinePower, err = s.EffectiveBaselinePower()
	if err != nil {
		return xerrors.Errorf("getting effective baseline power (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.totalMinedReward, err = s.TotalStoragePowerReward()
	if err != nil {
		return xerrors.Errorf("getting  total mined (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.newBaselinePower, err = s.ThisEpochBaselinePower()
	if err != nil {
		return xerrors.Errorf("getting this epoch baseline power (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.newBaseReward, err = s.ThisEpochReward()
	if err != nil {
		return xerrors.Errorf("getting this epoch baseline power (@ %s): %w", rw.common.stateroot.String(), err)
	}

	rw.newSmoothingEstimate, err = s.ThisEpochRewardSmoothed()
	if err != nil {
		return xerrors.Errorf("getting this epoch baseline power (@ %s): %w", rw.common.stateroot.String(), err)
	}
	return nil
}

func (p *Processor) setupRewards() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
/* captures chain-specific power state for any given stateroot */
create table if not exists chain_reward
(
	state_root text not null
		constraint chain_reward_pk
			primary key,
	cum_sum_baseline text not null,
	cum_sum_realized text not null,
	effective_network_time int not null,
	effective_baseline_power text not null,

	new_baseline_power text not null,
	new_reward numeric not null,
	new_reward_smoothed_position_estimate text not null,
	new_reward_smoothed_velocity_estimate text not null,

	total_mined_reward text not null
);
`); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *Processor) HandleRewardChanges(ctx context.Context, rewardTips ActorTips, nullRounds []types.TipSetKey) error {
	rewardChanges, err := p.processRewardActors(ctx, rewardTips, nullRounds)
	if err != nil {
		return xerrors.Errorf("Failed to process reward actors: %w", err)
	}

	if err := p.persistRewardActors(ctx, rewardChanges); err != nil {
		return err
	}

	return nil
}

func (p *Processor) processRewardActors(ctx context.Context, rewardTips ActorTips, nullRounds []types.TipSetKey) ([]rewardActorInfo, error) {
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
			rewardActor, err := p.node.StateGetActor(ctx, reward.Address, tipset)
			if err != nil {
				return nil, xerrors.Errorf("get reward state (@ %s): %w", rw.common.stateroot.String(), err)
			}

			rewardActorState, err := reward.Load(cw_util.NewAPIIpldStore(ctx, p.node), rewardActor)
			if err != nil {
				return nil, xerrors.Errorf("read state obj (@ %s): %w", rw.common.stateroot.String(), err)
			}
			if err := rw.set(rewardActorState); err != nil {
				return nil, err
			}

			out = append(out, rw)
		}
	}
	for _, tsKey := range nullRounds {
		var rw rewardActorInfo
		tipset, err := p.node.ChainGetTipSet(ctx, tsKey)
		if err != nil {
			return nil, err
		}
		rw.common.tsKey = tipset.Key()
		rw.common.height = tipset.Height()
		rw.common.stateroot = tipset.ParentState()
		rw.common.parentTsKey = tipset.Parents()
		// get reward actor states at each tipset once for all updates
		rewardActor, err := p.node.StateGetActor(ctx, reward.Address, tsKey)
		if err != nil {
			return nil, err
		}

		rewardActorState, err := reward.Load(cw_util.NewAPIIpldStore(ctx, p.node), rewardActor)
		if err != nil {
			return nil, xerrors.Errorf("read state obj (@ %s): %w", rw.common.stateroot.String(), err)
		}

		if err := rw.set(rewardActorState); err != nil {
			return nil, err
		}
		out = append(out, rw)
	}

	return out, nil
}

func (p *Processor) persistRewardActors(ctx context.Context, rewards []rewardActorInfo) error {
	start := time.Now()
	defer func() {
		log.Debugw("Persisted Reward Actors", "duration", time.Since(start).String())
	}()

	tx, err := p.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin chain_reward tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table cr (like chain_reward excluding constraints) on commit drop`); err != nil {
		return xerrors.Errorf("prep chain_reward temp: %w", err)
	}

	stmt, err := tx.Prepare(`copy cr ( state_root, cum_sum_baseline, cum_sum_realized, effective_network_time, effective_baseline_power, new_baseline_power, new_reward, new_reward_smoothed_position_estimate, new_reward_smoothed_velocity_estimate, total_mined_reward) from STDIN`)
	if err != nil {
		return xerrors.Errorf("prepare tmp chain_reward: %w", err)
	}

	for _, rewardState := range rewards {
		if _, err := stmt.Exec(
			rewardState.common.stateroot.String(),
			rewardState.cumSumBaselinePower.String(),
			rewardState.cumSumRealizedPower.String(),
			uint64(rewardState.effectiveNetworkTime),
			rewardState.effectiveBaselinePower.String(),
			rewardState.newBaselinePower.String(),
			rewardState.newBaseReward.String(),
			rewardState.newSmoothingEstimate.PositionEstimate.String(),
			rewardState.newSmoothingEstimate.VelocityEstimate.String(),
			rewardState.totalMinedReward.String(),
		); err != nil {
			log.Errorw("failed to store chain power", "state_root", rewardState.common.stateroot, "error", err)
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("close prepared chain_reward: %w", err)
	}

	if _, err := tx.Exec(`insert into chain_reward select * from cr on conflict do nothing`); err != nil {
		return xerrors.Errorf("insert chain_reward from tmp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit chain_reward tx: %w", err)
	}

	return nil
}
