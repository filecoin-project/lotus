package sealing

import (
	"context"

	"github.com/filecoin-project/lotus/chain/actors/policy"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/go-state-types/abi"
)

type PreCommitPolicy interface {
	Expiration(ctx context.Context, ps ...Piece) (abi.ChainEpoch, error)
}

type Chain interface {
	ChainHead(ctx context.Context) (TipSetToken, abi.ChainEpoch, error)
	StateNetworkVersion(ctx context.Context, tok TipSetToken) (network.Version, error)
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
	api Chain

	provingBoundary abi.ChainEpoch
	duration        abi.ChainEpoch
}

// NewBasicPreCommitPolicy produces a BasicPreCommitPolicy.
//
// The provided duration is used as the default sector expiry when the sector
// contains no deals. The proving boundary is used to adjust/align the sector's expiration.
func NewBasicPreCommitPolicy(api Chain, duration abi.ChainEpoch, provingBoundary abi.ChainEpoch) BasicPreCommitPolicy {
	return BasicPreCommitPolicy{
		api:             api,
		provingBoundary: provingBoundary,
		duration:        duration,
	}
}

// Expiration produces the pre-commit sector expiration epoch for an encoded
// replica containing the provided enumeration of pieces and deals.
func (p *BasicPreCommitPolicy) Expiration(ctx context.Context, ps ...Piece) (abi.ChainEpoch, error) {
	_, epoch, err := p.api.ChainHead(ctx)
	if err != nil {
		return 0, err
	}

	var hardMinimum *abi.ChainEpoch

	for _, p := range ps {
		if p.DealInfo == nil {
			continue
		}

		if p.DealInfo.DealSchedule.EndEpoch < epoch {
			log.Warnf("piece schedule %+v ended before current epoch %d", p, epoch)
			continue
		}

		if hardMinimum == nil || *hardMinimum < p.DealInfo.DealSchedule.EndEpoch {
			tmp := p.DealInfo.DealSchedule.EndEpoch
			hardMinimum = &tmp
		}
	}

	exp := epoch + p.duration
	if hardMinimum != nil {
		exp = *hardMinimum
	}

	// TODO: Is this still needed?
	// align on proving period boundary
	exp += miner.WPoStProvingPeriod - (exp % miner.WPoStProvingPeriod) + p.provingBoundary - 1

	// add / subtract out miner.WPoStProvingPeriod to allow 24h for the PC message to land, should probably be a config
	maxEnd := epoch + policy.GetMaxSectorExpirationExtension() - miner.WPoStProvingPeriod
	minEnd := epoch + policy.GetMinSectorExpiration() + miner.WPoStProvingPeriod

	if exp > maxEnd {
		// we need to subtract out ceil(end - maxEnd) days
		daysToSubtract := (exp-maxEnd)/miner.WPoStProvingPeriod + 1
		exp -= daysToSubtract * miner.WPoStProvingPeriod
	}

	if exp < minEnd {
		// we need to add ceil(end - maxEnd) days
		daysToAdd := (minEnd-exp)/miner.WPoStProvingPeriod + 1
		exp += daysToAdd * miner.WPoStProvingPeriod
	}

	if hardMinimum != nil && exp < *hardMinimum {
		// i guess the best thing to do here is to just set it to the deal-enforced limit?
		exp = *hardMinimum
	}

	return exp, nil
}
