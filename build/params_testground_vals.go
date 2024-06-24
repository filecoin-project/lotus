//go:build testground
// +build testground

package build

import (
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// Actor consts
// TODO: pieceSize unused from actors
var MinDealDuration, MaxDealDuration = policy.DealDurationBounds(0)

func init() {
	policy.SetSupportedProofTypes(buildconstants.SupportedProofTypes...)
	policy.SetConsensusMinerMinPower(buildconstants.ConsensusMinerMinPower)
	policy.SetMinVerifiedDealSize(buildconstants.MinVerifiedDealSize)
	policy.SetPreCommitChallengeDelay(buildconstants.PreCommitChallengeDelay)
}
