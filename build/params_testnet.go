// +build !debug
// +build !2k
// +build !testground

package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0: DrandMainnet,
}

const UpgradeBreezeHeight = -1
const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight = -1

func init() {
	// Minimum block production power is set to 4 TiB
	// Rationale is to discourage small-scale miners from trying to take over the network
	// One needs to invest in ~2.3x the compute to break consensus, making it not worth it
	//
	// DOWNSIDE: the fake-seals need to be kept alive/protected, otherwise network will seize
	//
	power.ConsensusMinerMinPower = big.NewInt(4 << 40)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg512MiBV1: {},
		abi.RegisteredSealProof_StackedDrg32GiBV1:  {},
		abi.RegisteredSealProof_StackedDrg64GiBV1:  {},
	}
	miner.PreCommitChallengeDelay = abi.ChainEpoch(10)
}

const BlockDelaySecs = uint64(builtin.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)
