package kit

import (
	"fmt"
	"os"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")

	// These values mimic the values set in the builtin-actors when configured to use the "testing" network. Specifically:
	// - All proof types.
	// - 2k minimum power.
	// - "small" verified deals.
	// - short precommit
	//
	// See:
	// - https://github.com/filecoin-project/builtin-actors/blob/0502c0722225ee58d1e6641431b4f9356cb2d18e/actors/runtime/src/runtime/policy.rs#L235
	// - https://github.com/filecoin-project/builtin-actors/blob/0502c0722225ee58d1e6641431b4f9356cb2d18e/actors/runtime/build.rs#L17-L45
	policy.SetProviderCollateralSupplyTarget(big.Zero(), big.NewInt(1))
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
	policy.SetPreCommitChallengeDelay(10)
	policy.SetSupportedProofTypes(
		abi.RegisteredSealProof_StackedDrg2KiBV1,
		abi.RegisteredSealProof_StackedDrg8MiBV1,
		abi.RegisteredSealProof_StackedDrg512MiBV1,
		abi.RegisteredSealProof_StackedDrg32GiBV1,
		abi.RegisteredSealProof_StackedDrg64GiBV1,
	)
	policy.SetProviderCollateralSupplyTarget(big.NewInt(0), big.NewInt(100))

	build.InsecurePoStValidation = true

	if err := os.Setenv("BELLMAN_NO_GPU", "1"); err != nil {
		panic(fmt.Sprintf("failed to set BELLMAN_NO_GPU env variable: %s", err))
	}

	if err := os.Setenv("LOTUS_DISABLE_WATCHDOG", "1"); err != nil {
		panic(fmt.Sprintf("failed to set LOTUS_DISABLE_WATCHDOG env variable: %s", err))
	}
}
