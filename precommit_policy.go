package sealing

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
)

type PreCommitPolicy interface {
	Expiration(ctx context.Context, pdis ...PieceWithOptionalDealInfo) (abi.ChainEpoch, error)
}

type Chain interface {
	ChainHead(ctx context.Context) (TipSetToken, abi.ChainEpoch, error)
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

	duration abi.ChainEpoch
}

// NewBasicPreCommitPolicy produces a BasicPreCommitPolicy
func NewBasicPreCommitPolicy(api Chain, duration abi.ChainEpoch) BasicPreCommitPolicy {
	return BasicPreCommitPolicy{
		api:      api,
		duration: duration,
	}
}

// Expiration produces the pre-commit sector expiration epoch for an encoded
// replica containing the provided enumeration of pieces and deals.
func (p *BasicPreCommitPolicy) Expiration(ctx context.Context, pdis ...PieceWithOptionalDealInfo) (abi.ChainEpoch, error) {
	_, epoch, err := p.api.ChainHead(ctx)
	if err != nil {
		return 0, nil
	}

	var end *abi.ChainEpoch

	for _, p := range pdis {
		if p.DealInfo == nil {
			continue
		}

		if p.DealInfo.DealSchedule.EndEpoch < epoch {
			log.Warnf("piece schedule %+v ended before current epoch %d", p, epoch)
			continue
		}

		if end == nil || *end < p.DealInfo.DealSchedule.EndEpoch {
			end = &p.DealInfo.DealSchedule.EndEpoch
		}
	}

	if end == nil {
		tmp := epoch + p.duration
		end = &tmp
	}

	return *end, nil
}
