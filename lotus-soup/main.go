package main

import (
	"os"

	"github.com/filecoin-project/oni/lotus-soup/paych"
	"github.com/filecoin-project/oni/lotus-soup/testkit"

	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	"github.com/testground/sdk-go/run"

	logging "github.com/ipfs/go-log/v2"
)

var cases = map[string]interface{}{
	"deals-e2e":         testkit.WrapTestEnvironment(dealsE2E),
	"deals-stress-test": testkit.WrapTestEnvironment(dealStressTest),
	"drand-halting":     testkit.WrapTestEnvironment(dealsE2E),
	"paych-stress":      testkit.WrapTestEnvironment(paych.Stress),
}

func init() {
	build.BlockDelaySecs = 2
	build.PropagationDelaySecs = 4

	_ = logging.SetLogLevel("*", "WARN")
	_ = logging.SetLogLevel("dht/RtRefreshManager", "ERROR") // noisy
	_ = logging.SetLogLevel("bitswap", "ERROR")              // noisy

	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	build.InsecurePoStValidation = true
	build.DisableBuiltinAssets = true

	// MessageConfidence is the amount of tipsets we wait after a message is
	// mined, e.g. payment channel creation, to be considered committed.
	build.MessageConfidence = 1

	power.ConsensusMinerMinPower = big.NewInt(2048)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)

}

func main() {
	run.InvokeMap(cases)
}
