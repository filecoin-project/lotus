package main

import (
	"os"

	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	"github.com/ipfs/go-log/v2"
)

func init() {
	build.BlockDelaySecs = 2
	build.PropagationDelaySecs = 1

	_ = log.SetLogLevel("*", "WARN")
	_ = log.SetLogLevel("dht/RtRefreshManager", "ERROR") // noisy
	_ = log.SetLogLevel("bitswap", "ERROR")              // noisy

	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	build.InsecurePoStValidation = true
	build.DisableBuiltinAssets = true

	// MessageConfidence is the amount of tipsets we wait after a message is
	// mined, e.g. payment channel creation, to be considered committed.
	build.MessageConfidence = 1

	// The period over which all a miner's active sectors will be challenged.
	miner.WPoStProvingPeriod = abi.ChainEpoch(240) // instead of 24 hours

	// The duration of a deadline's challenge window, the period before a deadline when the challenge is available.
	miner.WPoStChallengeWindow = abi.ChainEpoch(5) // instead of 30 minutes (still 48 per day)

	// Number of epochs between publishing the precommit and when the challenge for interactive PoRep is drawn
	// used to ensure it is not predictable by miner.
	miner.PreCommitChallengeDelay = abi.ChainEpoch(10)

	power.ConsensusMinerMinPower = big.NewInt(2048)
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}
