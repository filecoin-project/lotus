package sealing

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type PreCommitPolicy interface {
	Expiration(ctx context.Context, ps ...SafeSectorPiece) (abi.ChainEpoch, error)
}

type Chain interface {
	ChainHead(context.Context) (*types.TipSet, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
}

// BasicPreCommitPolicy satisfies PreCommitPolicy. It has two modes:
//
// Mode 1: The sector contains a non-zero quantity of pieces with deal info
// Mode 2: The sector contains no pieces with deal info
//
// The BasicPreCommitPolicy#Expiration method is given a slice of the pieces
// which the miner has encoded into the sector, and from that slice picks either
// the first or second mode.
//
// If we're in Mode 1: The pre-commit expiration epoch will be the maximum
// deal end epoch of a piece in the sector.
//
// If we're in Mode 2: The pre-commit expiration epoch will be set to the
// current epoch + the provided default duration.
type BasicPreCommitPolicy struct {
	api              Chain
	getSealingConfig dtypes.GetSealingConfigFunc

	provingBuffer abi.ChainEpoch
}

// NewBasicPreCommitPolicy produces a BasicPreCommitPolicy.
//
// The provided duration is used as the default sector expiry when the sector
// contains no deals. The proving boundary is used to adjust/align the sector's expiration.
func NewBasicPreCommitPolicy(api Chain, cfgGetter dtypes.GetSealingConfigFunc, provingBuffer abi.ChainEpoch) BasicPreCommitPolicy {
	return BasicPreCommitPolicy{
		api:              api,
		getSealingConfig: cfgGetter,
		provingBuffer:    provingBuffer,
	}
}

// Expiration produces the pre-commit sector expiration epoch for an encoded
// replica containing the provided enumeration of pieces and deals.
func (p *BasicPreCommitPolicy) Expiration(ctx context.Context, ps ...SafeSectorPiece) (abi.ChainEpoch, error) {
	ts, err := p.api.ChainHead(ctx)
	if err != nil {
		return 0, err
	}

	var end *abi.ChainEpoch

	for _, p := range ps {
		if !p.HasDealInfo() {
			continue
		}

		endEpoch, err := p.EndEpoch()
		if err != nil {
			return 0, xerrors.Errorf("failed to get end epoch: %w", err)
		}

		if endEpoch < ts.Height() {
			log.Warnf("piece schedule %+v ended before current epoch %d", p, ts.Height())
			continue
		}

		if end == nil || *end < endEpoch {
			tmp := endEpoch
			end = &tmp
		}
	}

	if end == nil {
		nv, err := p.api.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return 0, xerrors.Errorf("failed to get network version: %w", err)
		}

		// no deal pieces, get expiration for committed capacity sector
		expirationDuration, err := p.getCCSectorLifetime(nv)
		if err != nil {
			return 0, xerrors.Errorf("failed to get cc sector lifetime: %w", err)
		}

		tmp := ts.Height() + expirationDuration
		end = &tmp
	}

	// Ensure there is at least one day for the PC message to land without falling below min sector lifetime
	// TODO: The "one day" should probably be a config, though it doesn't matter too much
	minExp := ts.Height() + policy.GetMinSectorExpiration() + miner.WPoStProvingPeriod
	if *end < minExp {
		end = &minExp
	}

	return *end, nil
}

func (p *BasicPreCommitPolicy) getCCSectorLifetime(nv network.Version) (abi.ChainEpoch, error) {
	c, err := p.getSealingConfig()
	if err != nil {
		return 0, xerrors.Errorf("sealing config load error: %w", err)
	}

	maxCommitment, err := policy.GetMaxSectorExpirationExtension(nv)
	if err != nil {
		return 0, xerrors.Errorf("failed to get max extension: %w", err)
	}

	var ccLifetimeEpochs = abi.ChainEpoch(uint64(c.CommittedCapacitySectorLifetime.Seconds()) / builtin.EpochDurationSeconds)
	// if zero value in config, assume default sector extension
	if ccLifetimeEpochs == 0 {
		ccLifetimeEpochs = maxCommitment
	}

	if minExpiration := policy.GetMinSectorExpiration(); ccLifetimeEpochs < minExpiration {
		log.Warnf("value for CommittedCapacitySectorLifetime is too short, using default minimum (%d epochs)", minExpiration)
		return minExpiration, nil
	}
	if ccLifetimeEpochs > maxCommitment {
		log.Warnf("value for CommittedCapacitySectorLifetime is too long, using default maximum (%d epochs)", maxCommitment)
		return maxCommitment, nil
	}

	return ccLifetimeEpochs - p.provingBuffer, nil
}
