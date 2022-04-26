//go:build debug
// +build debug

package build

func init() {
	InsecurePoStValidation = true
	BuildType |= BuildDebug
}

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20
