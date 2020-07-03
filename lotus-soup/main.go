package main

import (
	"github.com/testground/sdk-go/run"

	"github.com/filecoin-project/oni/lotus-soup/testkit"

	"github.com/filecoin-project/lotus/build"
)

var cases = map[string]interface{}{
	"deals-e2e":         testkit.WrapTestEnvironment(dealsE2E),
	"deals-stress-test": testkit.WrapTestEnvironment(dealStressTest),
	"drand-halting":     testkit.WrapTestEnvironment(dealsE2E),
}

func init() {
	build.BlockDelaySecs = 1
	build.PropagationDelaySecs = 2
}

func main() {
	run.InvokeMap(cases)
}
