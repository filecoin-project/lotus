// +build !debug

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var SectorSizes = []abi.SectorSize{
	512 << 20,
	32 << 30,
}

// Seconds
const BlockDelay = 25

const PropagationDelay = 6
