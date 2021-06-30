package itests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestDealWithMarketAndMinerNode(t *testing.T) {
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

	// TODO: add 2, 4, 8, more when this graphsync issue is fixed: https://github.com/ipfs/go-graphsync/issues/175#
	cycles := []int{1}
	for _, n := range cycles {
		n := n
		ns := fmt.Sprintf("%d", n)
		t.Run(ns+"-fastretrieval-CAR", func(t *testing.T) { runTest(t, n, true, true) })
		//t.Run(ns+"-fastretrieval-NoCAR", func(t *testing.T) { runTest(t, n, true, false) })
		//t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, true, false) })
		//t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
	}
}

func TestDealCyclesConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	blockTime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T, n int, fastRetrieval bool, carExport bool) {
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
		ens.InterconnectAll().BeginMining(blockTime)
		dh := kit.NewDealHarness(t, client, miner, miner)

		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{
			N:             n,
			FastRetrieval: fastRetrieval,
			CarExport:     carExport,
			StartEpoch:    startEpoch,
		})
	}

	// TODO: add 2, 4, 8, more when this graphsync issue is fixed: https://github.com/ipfs/go-graphsync/issues/175#
	cycles := []int{1}
	for _, n := range cycles {
		n := n
		ns := fmt.Sprintf("%d", n)
		t.Run(ns+"-fastretrieval-CAR", func(t *testing.T) { runTest(t, n, true, true) })
		t.Run(ns+"-fastretrieval-NoCAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
	}
}

func TestSimultenousTransferLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	blockTime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T) {
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ConstructorOpts(
			node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.StagingGraphsync), modules.StagingGraphsync(2))),
		))
		ens.InterconnectAll().BeginMining(blockTime)
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
					if u.Status == datatransfer.Ongoing {
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

		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{
			N:             1, // TODO: set to 20 after https://github.com/ipfs/go-graphsync/issues/175 is fixed
			FastRetrieval: true,
			StartEpoch:    startEpoch,
		})

		cancel()
		wg.Wait()

		require.LessOrEqual(t, maxOngoing, 2)
	}

	runTest(t)
}
