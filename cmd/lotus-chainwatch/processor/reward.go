package processor

import (
	"bytes"
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
)

type rewardActorInfo struct {
	common actorInfo

	baselinePower big.Int
}

func (p *Processor) setupRewards() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
/*
* captures chain-specific power state for any given stateroot
*/
create table if not exists chain_power
(
	state_root text not null
		constraint chain_power_pk
			primary key,
	baseline_power text not null
);
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
		log.Infow("Processed Reward Actors", "duration", time.Since(start).String())
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

			rw.baselinePower = rewardActorState.BaselinePower
			out = append(out, rw)
		}
	}
	return out, nil
}

func (p *Processor) persistRewardActors(ctx context.Context, rewards []rewardActorInfo) error {
	start := time.Now()
	defer func() {
		log.Infow("Persisted Reward Actors", "duration", time.Since(start).String())
	}()

	if err := p.storeChainPower(rewards); err != nil {
		return err
	}

	return nil
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
