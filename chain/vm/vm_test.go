package vm

import (
	"fmt"
	"testing"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/types"
)

func TestBlockReward(t *testing.T) {
	sum := types.NewInt(0)
	for i := 0; i < 500000000; i += 60 {
		sum = types.BigAdd(sum, MiningRewardForBlock(uint64(i)))
	}

	sum = types.BigMul(sum, types.NewInt(60))

	fmt.Println(sum)

	fmt.Println(types.BigDiv(sum, types.NewInt(build.FilecoinPrecision)))
}
