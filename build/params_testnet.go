// +build !debug
// +build !2k

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

func init() {
	power.ConsensusMinerMinPower = big.NewInt(1024 << 30)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg32GiBV1: {},
		abi.RegisteredSealProof_StackedDrg64GiBV1: {},
	}
}

// Seconds
const BlockDelay = builtin.EpochDurationSeconds

const PropagationDelay = 6
