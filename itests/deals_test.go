package itests

import (
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestDealsWithSealingAndRPC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	var blockTime = 1 * time.Second

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC()) // no mock proofs.
	ens.InterconnectAll().BeginMining(blockTime)
	dh := kit.NewDealHarness(t, client, miner)

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
}
