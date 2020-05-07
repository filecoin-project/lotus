// +build !debug
// +build !2k

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

func init() {
	power.ConsensusMinerMinPower = big.NewInt(2 << 30)
}

var SectorSizes = []abi.SectorSize{
	512 << 20,
	32 << 30,
}

// Seconds
const BlockDelay = 25

const PropagationDelay = 6
