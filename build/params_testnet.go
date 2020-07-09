// +build !debug
// +build !2k
// +build !testground

package build

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

func init() {
	power.ConsensusMinerMinPower = big.NewInt(1024 << 20)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg512MiBV1: {},
		abi.RegisteredSealProof_StackedDrg32GiBV1:  {},
		abi.RegisteredSealProof_StackedDrg64GiBV1:  {},
	}
}

const BlockDelaySecs = uint64(builtin.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)
