// +build !debug

package build

// Seconds
const BlockDelay = 30

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
