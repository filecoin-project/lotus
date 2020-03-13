// +build !debug

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

var SectorSizes = []abi.SectorSize{
	512 << 20,
	32 << 30,
}

// Seconds
const BlockDelay = 25

const PropagationDelay = 6

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = miner.ProvingPeriod * 3 // TODO: remove

// Epochs
const InteractivePoRepConfidence = 6
