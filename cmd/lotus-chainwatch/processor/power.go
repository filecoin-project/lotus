package processor

import (
	"bytes"
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/smoothing"
)

type powerActorInfo struct {
	common actorInfo

	epochSmoothingEstimate *smoothing.FilterEstimate
}

func (p *Processor) setupPower() error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
create table if not exists power_smoothing_estimates
(
    state_root text not null
        constraint power_smoothing_estimates_pk
        	primary key,
	position_estimate text not null,
	velocity_estimate text not null
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
	for tipset, powers := range powerTips {
		for _, act := range powers {
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

			pw.epochSmoothingEstimate = powerActorState.ThisEpochQAPowerSmoothed
			out = append(out, pw)
		}
	}

	return out, nil
}

func (p *Processor) persistPowerActors(ctx context.Context, powers []powerActorInfo) error {
	// NB: use errgroup when there is more than a single store operation
	return p.storePowerSmoothingEstimates(powers)
}

func (p *Processor) storePowerSmoothingEstimates(powers []powerActorInfo) error {
	tx, err := p.db.Begin()
	if err != nil {
		return xerrors.Errorf("begin power_smoothing_estimates tx: %w", err)
	}

	if _, err := tx.Exec(`create temp table rse (like power_smoothing_estimates) on commit drop`); err != nil {
		return xerrors.Errorf("prep power_smoothing_estimates: %w", err)
	}

	stmt, err := tx.Prepare(`copy rse (state_root, position_estimate, velocity_estimate) from stdin;`)
	if err != nil {
		return xerrors.Errorf("prepare tmp power_smoothing_estimates: %w", err)
	}

	for _, powerState := range powers {
		if _, err := stmt.Exec(
			powerState.common.stateroot.String(),
			powerState.epochSmoothingEstimate.PositionEstimate.String(),
			powerState.epochSmoothingEstimate.VelocityEstimate.String(),
		); err != nil {
			return xerrors.Errorf("failed to store smoothing estimate: %w", err)
		}
	}

	if err := stmt.Close(); err != nil {
		return xerrors.Errorf("close prepared power_smoothing_estimates: %w", err)
	}

	if _, err := tx.Exec(`insert into power_smoothing_estimates select * from rse on conflict do nothing`); err != nil {
		return xerrors.Errorf("insert power_smoothing_estimates from tmp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit power_smoothing_estimates tx: %w", err)
	}

	return nil

}
