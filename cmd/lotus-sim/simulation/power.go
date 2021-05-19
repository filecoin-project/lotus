package simulation

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
)

type powerInfo struct {
	powerLookback, powerNow abi.StoragePower
}

// Load all power claims at the given height.
func (sim *Simulation) loadClaims(ctx context.Context, height abi.ChainEpoch) (map[address.Address]power.Claim, error) {
	powerTable := make(map[address.Address]power.Claim)
	store := sim.Chainstore.ActorStore(ctx)

	ts, err := sim.Chainstore.GetTipsetByHeight(ctx, height, sim.head, true)
	if err != nil {
		return nil, xerrors.Errorf("when projecting growth, failed to lookup lookback epoch: %w", err)
	}

	powerActor, err := sim.sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return nil, err
	}

	powerState, err := power.Load(store, powerActor)
	if err != nil {
		return nil, err
	}
	err = powerState.ForEachClaim(func(miner address.Address, claim power.Claim) error {
		powerTable[miner] = claim
		return nil
	})
	if err != nil {
		return nil, err
	}
	return powerTable, nil
}

// Compute the number of sectors a miner has from their power claim.
func sectorsFromClaim(sectorSize abi.SectorSize, c power.Claim) int64 {
	if c.RawBytePower.Int == nil {
		return 0
	}
	sectorCount := big.Div(c.RawBytePower, big.NewIntUnsigned(uint64(sectorSize)))
	if !sectorCount.IsInt64() {
		panic("impossible number of sectors")
	}
	return sectorCount.Int64()
}
