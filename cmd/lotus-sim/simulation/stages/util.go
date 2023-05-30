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

func postChainCommitInfo(ctx context.Context, bb *blockbuilder.BlockBuilder, epoch abi.ChainEpoch) (abi.Randomness, error) {
	ts := bb.ParentTipSet()
	commitRand, err := bb.StateManager().GetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, epoch, nil, ts.Key())
	return commitRand, err
}
