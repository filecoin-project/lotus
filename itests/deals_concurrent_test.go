package itests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

// TestDealWithMarketAndMinerNode is running concurrently a number of storage and retrieval deals towards a miner
// architecture where the `mining/sealing/proving` node is a separate process from the `markets` node
func TestDealWithMarketAndMinerNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	t.Skip("skipping due to flakiness: see #6956")

	kit.QuietMiningLogs()

	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(8 << 10)

	runTest := func(t *testing.T, n int, fastRetrieval bool, carExport bool) {
		api.RunningNodeType = api.NodeMiner // TODO(anteva): fix me

		client, main, market, _ := kit.EnsembleWithMinerAndMarketNodes(t, kit.ThroughRPC())

		dh := kit.NewDealHarness(t, client, main, market)

		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{
			N:             n,
			FastRetrieval: fastRetrieval,
			CarExport:     carExport,
			StartEpoch:    startEpoch,
		})
	}

	// this test is expensive because we don't use mock proofs; do a single cycle.
	cycles := []int{4}
	for _, n := range cycles {
		n := n
		ns := fmt.Sprintf("%d", n)
		t.Run(ns+"-fastretrieval-CAR", func(t *testing.T) { runTest(t, n, true, true) })
		t.Run(ns+"-fastretrieval-NoCAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, false, true) })
		t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
	}
}

func TestDealCyclesConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	kit.QuietMiningLogs()

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T, n int, fastRetrieval bool, carExport bool) {
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
		ens.InterconnectAll().BeginMining(250 * time.Millisecond)
		dh := kit.NewDealHarness(t, client, miner, miner)

		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{
			N:             n,
			FastRetrieval: fastRetrieval,
			CarExport:     carExport,
			StartEpoch:    startEpoch,
		})
	}

	// this test is cheap because we use mock proofs, do various cycles
	cycles := []int{2, 4, 8, 16}
	for _, n := range cycles {
		n := n
		ns := fmt.Sprintf("%d", n)
		t.Run(ns+"-fastretrieval-CAR", func(t *testing.T) { runTest(t, n, true, true) })
		t.Run(ns+"-fastretrieval-NoCAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, false, true) })
		t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
	}
}

func TestSimultanenousTransferLimit(t *testing.T) {
	t.Skip("skipping as flaky #7152")

	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	const (
		graphsyncThrottle = 2
		concurrency       = 20
	)
	runTest := func(t *testing.T) {
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ConstructorOpts(
			node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.StagingGraphsync), modules.StagingGraphsync(graphsyncThrottle, graphsyncThrottle))),
			node.Override(new(dtypes.Graphsync), modules.Graphsync(graphsyncThrottle, graphsyncThrottle)),
		))
		ens.InterconnectAll().BeginMining(250 * time.Millisecond)
		dh := kit.NewDealHarness(t, client, miner, miner)

		ctx, cancel := context.WithCancel(context.Background())

		du, err := miner.MarketDataTransferUpdates(ctx)
		require.NoError(t, err)

		var maxOngoing int
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()

			ongoing := map[datatransfer.TransferID]struct{}{}

			for {
				select {
				case u := <-du:
					t.Logf("%d - %s", u.TransferID, datatransfer.Statuses[u.Status])
					if u.Status == datatransfer.Ongoing && u.Transferred > 0 {
						ongoing[u.TransferID] = struct{}{}
					} else {
						delete(ongoing, u.TransferID)
					}

					if len(ongoing) > maxOngoing {
						maxOngoing = len(ongoing)
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		t.Logf("running concurrent deals: %d", concurrency)

		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{
			N:             concurrency,
			FastRetrieval: true,
			StartEpoch:    startEpoch,
		})

		t.Logf("all deals finished")

		cancel()
		wg.Wait()

		// The eventing systems across go-data-transfer and go-graphsync
		// are racy, and that's why we can't enforce graphsyncThrottle exactly,
		// without making this test racy.
		//
		// Essentially what could happen is that the graphsync layer starts the
		// next transfer before the go-data-transfer FSM has the opportunity to
		// move the previously completed transfer to the next stage, thus giving
		// the appearance that more than graphsyncThrottle transfers are
		// in progress.
		//
		// Concurrency (20) is x10 higher than graphsyncThrottle (2), so if all
		// 20 transfers are not happening at once, we know the throttle is
		// in effect. Thus we are a little bit lenient here to account for the
		// above races and allow up to graphsyncThrottle*2.
		require.LessOrEqual(t, maxOngoing, graphsyncThrottle*2)
	}

	runTest(t)
}
