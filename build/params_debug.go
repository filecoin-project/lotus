// +build debug

package build

import "os"

var SectorSizes = []uint64{1024}

// Seconds
const BlockDelay = 6

const PropagationDelay = 3

// FallbackPoStDelay is the number of epochs the miner needs to wait after
//  ElectionPeriodStart before starting fallback post computation
//
// Epochs
const FallbackPoStDelay = 10

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20

// Epochs
const InteractivePoRepDelay = 2

// Epochs
const InteractivePoRepConfidence = 6

// Bytes
const MinimumMinerPower = 2 << 10 // 2KiB

func init() {
	os.Setenv("TRUST_PARAMS", "1")
}
