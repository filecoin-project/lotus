package main

import (
	"fmt"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"

	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	logging "github.com/ipfs/go-log/v2"
)

var testplans = map[string]interface{}{
	"lotus-baseline": doRun(baselineRoles),
}

func init() {
	logging.SetLogLevel("vm", "WARN")
	logging.SetLogLevel("miner", "WARN")
	logging.SetLogLevel("chainstore", "WARN")
	logging.SetLogLevel("chain", "WARN")
	logging.SetLogLevel("sub", "WARN")
	logging.SetLogLevel("storageminer", "WARN")

	build.DisableBuiltinAssets = true

	// Note: I don't understand the significance of this, but the node test does it.
	power.ConsensusMinerMinPower = big.NewInt(2048)
	saminer.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}

func main() {
	run.InvokeMap(testplans)
}

func doRun(roles map[string]func(*TestEnvironment) error) run.InitializedTestCaseFn {
	return func(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
		role := runenv.StringParam("role")
		proc, ok := roles[role]
		if ok {
			return proc(&TestEnvironment{RunEnv: runenv, InitContext: initCtx})
		}
		return fmt.Errorf("Unknown role: %s", role)
	}
}
