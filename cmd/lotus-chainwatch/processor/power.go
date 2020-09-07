package processor

import (
	"bytes"
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/smoothing"
)

type powerActorInfo struct {
	common actorInfo

	totalRawBytes                      big.Int
	totalRawBytesCommitted             big.Int
	totalQualityAdjustedBytes          big.Int
	totalQualityAdjustedBytesCommitted big.Int
	totalPledgeCollateral              big.Int

	newRawBytes             big.Int
	newQualityAdjustedBytes big.Int
	newPledgeCollateral     big.Int
	newQAPowerSmoothed      *smoothing.FilterEstimate

	minerCount                  int64
	minerCountAboveMinimumPower int64
}

func (p *Processor) setupPower() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
create table if not exists chain_power
(
	state_root text not null
		constraint power_smoothing_estimates_pk
			primary key,

	new_raw_bytes_power text not null,
	new_qa_bytes_power text not null,
	new_pledge_collateral text not null,

	total_raw_bytes_power text not null,
	total_raw_bytes_committed text not null,
	total_qa_bytes_power text not null,
	total_qa_bytes_committed text not null,
	total_pledge_collateral text not null,

	qa_smoothed_position_estimate text not null,
	qa_smoothed_velocity_estimate text not null,

	miner_count int not null,
	minimum_consensus_miner_count int not null
);
`); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *Processor) HandlePowerChanges(ctx context.Context, powerTips ActorTips) error {
	powerChanges, err := p.processPowerActors(ctx, powerTips)
	if err != nil {
		return xerrors.Errorf("Failed to process power actors: %w", err)
	}

	if err := p.persistPowerActors(ctx, powerChanges); err != nil {
		return err
	}

	return nil
}

func (p *Processor) processPowerActors(ctx context.Context, powerTips ActorTips) ([]powerActorInfo, error) {
	start := time.Now()
	defer func() {
		log.Debugw("Processed Power Actors", "duration", time.Since(start).String())
	}()

	var out []powerActorInfo
	for tipset, powerStates := range powerTips {
		for _, act := range powerStates {
			var pw powerActorInfo
			pw.common = act

			powerActor, err := p.node.StateGetActor(ctx, builtin.StoragePowerActorAddr, tipset)
			if err != nil {
				return nil, xerrors.Errorf("get power state (@ %s): %w", pw.common.stateroot.String(), err)
			}

			powerStateRaw, err := p.node.ChainReadObj(ctx, powerActor.Head)
			if err != nil {
				return nil, xerrors.Errorf("read state obj (@ %s): %w", pw.common.stateroot.String(), err)
			}

			var powerActorState power.State
			if err := powerActorState.UnmarshalCBOR(bytes.NewReader(powerStateRaw)); err != nil {
				return nil, xerrors.Errorf("unmarshal state (@ %s): %w", pw.common.stateroot.String(), err)
			}

			pw.totalRawBytes = powerActorState.TotalRawBytePower
			pw.totalRawBytesCommitted = powerActorState.TotalBytesCommitted
			pw.totalQualityAdjustedBytes = powerActorState.TotalQualityAdjPower
			pw.totalQualityAdjustedBytesCommitted = powerActorState.TotalQABytesCommitted
			pw.totalPledgeCollateral = powerActorState.TotalPledgeCollateral

			pw.newRawBytes = powerActorState.ThisEpochRawBytePower
			pw.newQualityAdjustedBytes = powerActorState.ThisEpochQualityAdjPower
			pw.newPledgeCollateral = powerActorState.ThisEpochPledgeCollateral
			pw.newQAPowerSmoothed = powerActorState.ThisEpochQAPowerSmoothed

			pw.minerCount = powerActorState.MinerCount
			pw.minerCountAboveMinimumPower = powerActorState.MinerAboveMinPowerCount
			out = append(out, pw)
		}
	}

	return out, nil
}

func (p *Processor) persistPowerActors(ctx context.Context, powerStates []powerActorInfo) error {
	// NB: use errgroup when there is more than a single store operation
	return p.storePowerSmoothingEstimates(powerStates)
}

func (p *Processor) storePowerSmoothingEstimates(powerStates []powerActorInfo) error {
	tx, err := p.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin chain_power tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table cp (like chain_power) on commit drop`); err != nil {
		return xerrors.Errorf("prep chain_power: %w", err)
	}

	stmt, err := tx.Prepare(`copy cp (state_root, new_raw_bytes_power, new_qa_bytes_power, new_pledge_collateral, total_raw_bytes_power, total_raw_bytes_committed, total_qa_bytes_power, total_qa_bytes_committed, total_pledge_collateral, qa_smoothed_position_estimate, qa_smoothed_velocity_estimate, miner_count, minimum_consensus_miner_count) from stdin;`)
	if err != nil {
		return xerrors.Errorf("prepare tmp chain_power: %w", err)
	}

	for _, ps := range powerStates {
		if _, err := stmt.Exec(
			ps.common.stateroot.String(),
			ps.newRawBytes.String(),
			ps.newQualityAdjustedBytes.String(),
			ps.newPledgeCollateral.String(),

			ps.totalRawBytes.String(),
			ps.totalRawBytesCommitted.String(),
			ps.totalQualityAdjustedBytes.String(),
			ps.totalQualityAdjustedBytesCommitted.String(),
			ps.totalPledgeCollateral.String(),

			ps.newQAPowerSmoothed.PositionEstimate.String(),
			ps.newQAPowerSmoothed.VelocityEstimate.String(),

			ps.minerCount,
			ps.minerCountAboveMinimumPower,
		); err != nil {
			return xerrors.Errorf("failed to store smoothing estimate: %w", err)
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
