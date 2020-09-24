// +build !debug
// +build !2k
// +build !testground

package build

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/policy"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
)

var DrandSchedule = map[abi.ChainEpoch]DrandEnum{
	0:                  DrandIncentinet,
	UpgradeSmokeHeight: DrandMainnet,
}

const UpgradeBreezeHeight = 41280
const BreezeGasTampingDuration = 120

const UpgradeSmokeHeight = 51000

func init() {
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(10 << 40))
	policy.SetSupportedProofTypes(
		abi.RegisteredSealProof_StackedDrg32GiBV1,
		abi.RegisteredSealProof_StackedDrg64GiBV1,
	)
}

const BlockDelaySecs = uint64(builtin0.EpochDurationSeconds)

const PropagationDelaySecs = uint64(6)
