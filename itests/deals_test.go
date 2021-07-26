package itests

import (
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestDealsWithSealingAndRPC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.WithAllSubsystems()) // no mock proofs.
	ens.InterconnectAll().BeginMining(250 * time.Millisecond)
	dh := kit.NewDealHarness(t, client, miner, miner)

	t.Run("stdretrieval", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1})
	})

	t.Run("fastretrieval", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, FastRetrieval: true})
	})

	t.Run("fastretrieval-twodeals-sequential", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, FastRetrieval: true})
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, FastRetrieval: true})
	})

	t.Run("stdretrieval-carv1", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1, UseCARFileForStorageDeal: true})
	})

}
