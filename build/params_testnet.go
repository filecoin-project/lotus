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
const BlockDelay = 45

const PropagationDelay = 6

// FallbackPoStDelay is the number of epochs the miner needs to wait after
//  ElectionPeriodStart before starting fallback post computation
//
// Epochs
const FallbackPoStDelay = miner.ProvingPeriod

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = miner.ProvingPeriod * 3 // TODO: remove

// Epochs
const InteractivePoRepConfidence = 6

// Bytes
var MinimumMinerPower uint64 = 2 << 30 // 2 GiB
