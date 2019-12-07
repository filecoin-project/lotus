// +build !debug

package build

var SectorSizes = []uint64{
	16 << 20,
	256 << 20,
	1 << 30,
	32 << 30,
}

// Seconds
const BlockDelay = 30

const PropagationDelay = 5

// FallbackPoStDelay is the number of epochs the miner needs to wait after
//  ElectionPeriodStart before starting fallback post computation
//
// Epochs
const FallbackPoStDelay = 30

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 200

// Epochs
const InteractivePoRepDelay = 8

// Epochs
const InteractivePoRepConfidence = 6
