// +build debug

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
)

func init() {
	InsecurePoStValidation = true
}

var SectorSizes = []abi.SectorSize{2048}

// Seconds
const BlockDelay = 6

const PropagationDelay = 3

// SlashablePowerDelay is the number of epochs after ElectionPeriodStart, after
// which the miner is slashed
//
// Epochs
const SlashablePowerDelay = 20

// Epochs
const InteractivePoRepConfidence = 6
