package main

import (
	"github.com/testground/sdk-go/run"

	"github.com/filecoin-project/oni/lotus-soup/testkit"
)

var cases = map[string]interface{}{
	"deals-e2e":         testkit.WrapTestEnvironment(dealsE2E),
	"deals-stress-test": testkit.WrapTestEnvironment(dealStressTest),
}

func main() {
	run.InvokeMap(cases)
}
