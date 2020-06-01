// +build debug 2k

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
)

func init() {
	power.ConsensusMinerMinPower = big.NewInt(2048)
	miner.SupportedProofTypes = map[abi.RegisteredProof]struct{}{
		abi.RegisteredProof_StackedDRG2KiBSeal: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)

	BuildType |= Build2k
}

// Seconds
const BlockDelay = 2

const PropagationDelay = 3

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20

// Epochs
const InteractivePoRepConfidence = 6
