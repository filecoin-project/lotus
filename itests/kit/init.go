package kit

import (
	"fmt"
	"os"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	logging "github.com/ipfs/go-log/v2"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")

	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))

	build.InsecurePoStValidation = true

	if err := os.Setenv("BELLMAN_NO_GPU", "1"); err != nil {
		panic(fmt.Sprintf("failed to set BELLMAN_NO_GPU env variable: %s", err))
	}

	if err := os.Setenv("LOTUS_DISABLE_WATCHDOG", "1"); err != nil {
		panic(fmt.Sprintf("failed to set LOTUS_DISABLE_WATCHDOG env variable: %s", err))
	}
}
