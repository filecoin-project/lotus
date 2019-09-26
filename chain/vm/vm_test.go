package vm

import (
	"fmt"
	"testing"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/types"
)

func TestBlockReward(t *testing.T) {
	coffer := types.FromFil(build.MiningRewardTotal)
	sum := types.NewInt(0)
	N := build.HalvingPeriodBlocks
	for i := 0; i < N; i++ {
		a := MiningReward(coffer)
		sum = types.BigAdd(sum, a)
		coffer = types.BigSub(coffer, a)
	}

	//sum = types.BigMul(sum, types.NewInt(60))

	fmt.Println("After a halving period")
	fmt.Printf("Total reward: %d\n", build.MiningRewardTotal)
	fmt.Printf("Remaining: %s\n", types.BigDiv(coffer, types.NewInt(build.FilecoinPrecision)))
	fmt.Printf("Given out: %s\n", types.BigDiv(sum, types.NewInt(build.FilecoinPrecision)))
}
