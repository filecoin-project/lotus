package stages

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/blockbuilder"
)

func loadMiner(store adt.Store, st types.StateTree, addr address.Address) (miner.State, error) {
	minerActor, err := st.GetActor(addr)
	if err != nil {
		return nil, err
	}
	return miner.Load(store, minerActor)
}

func loadPower(store adt.Store, st types.StateTree) (power.State, error) {
	powerActor, err := st.GetActor(power.Address)
	if err != nil {
		return nil, err
	}
	return power.Load(store, powerActor)
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

// loadClaims will load all non-zero claims at the given epoch.
func loadClaims(
	ctx context.Context, bb *blockbuilder.BlockBuilder, height abi.ChainEpoch,
) (map[address.Address]power.Claim, error) {
	powerTable := make(map[address.Address]power.Claim)

	st, err := bb.StateTreeByHeight(height)
	if err != nil {
		return nil, err
	}

	powerState, err := loadPower(bb.ActorStore(), st)
	if err != nil {
		return nil, err
	}

	err = powerState.ForEachClaim(func(miner address.Address, claim power.Claim) error {
		// skip miners without power
		if claim.RawBytePower.IsZero() {
			return nil
		}
		powerTable[miner] = claim
		return nil
	})
	if err != nil {
		return nil, err
	}
	return powerTable, nil
}

func postChainCommitInfo(ctx context.Context, bb *blockbuilder.BlockBuilder, epoch abi.ChainEpoch) (abi.Randomness, error) {
	cs := bb.StateManager().ChainStore()
	ts := bb.ParentTipSet()
	commitRand, err := cs.GetChainRandomness(ctx, ts.Cids(), crypto.DomainSeparationTag_PoStChainCommit, epoch, nil, true)
	return commitRand, err
}
