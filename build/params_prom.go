// +build prom

package build

var SectorSizes = []uint64{1024}

// Seconds
const BlockDelay = 45

const PropagationDelay = 6

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
var MinimumMinerPower uint64 = 2 << 10 // 2KiB
