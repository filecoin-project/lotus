package vm

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"

	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

const HalvingPeriodEpochs = 6 * 365 * 24 * 60 * 2

func TestBlockReward(t *testing.T) {
	coffer := types.FromFil(build.MiningRewardTotal).Int
	sum := new(big.Int)
	N := HalvingPeriodEpochs
	for i := 0; i < N; i++ {
		a := MiningReward(types.BigInt{coffer})
		sum = sum.Add(sum, a.Int)
		coffer = coffer.Sub(coffer, a.Int)
	}

	//sum = types.BigMul(sum, types.NewInt(60))

	fmt.Println("After a halving period")
	fmt.Printf("Total reward: %d\n", build.MiningRewardTotal)
	fmt.Printf("Remaining: %s\n", types.BigDiv(types.BigInt{coffer}, types.NewInt(build.FilecoinPrecision)))
	fmt.Printf("Given out: %s\n", types.BigDiv(types.BigInt{sum}, types.NewInt(build.FilecoinPrecision)))
}
