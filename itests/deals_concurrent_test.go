package itests

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/dagstore"
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

// TestDealWithMarketAndMinerNode is running concurrently a number of storage and retrieval deals towards a miner
// architecture where the `mining/sealing/proving` node is a separate process from the `markets` node
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
		t.Run(ns+"-fastretrieval-NoCAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, false, true) })
		t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
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
		t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, false, true) })
		t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
	}
}

// TestDealCycleProviderRestart verifies that
// - the client makes a deal with the storage provider
// - the storage provider restarts
// - the client can make a retrieval deal with the storage provider
func TestDealCycleProviderRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	blockTime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	// To simulate restarting the provider, we create two storage providers,
	// and share a DAG store between them.
	// We make a storage deal against the first provider, then start the second
	// provider with the same DAG store and make a retrieval deal aaginst the
	// second provider.
	var (
		client    kit.TestFullNode
		provider1 kit.TestMiner
		provider2 kit.TestMiner
	)

	// Use a single DAG store shared between miner1 and miner2
	dagStoreDir := t.TempDir()
	dagStoreDS := ds_sync.MutexWrap(ds.NewMapDatastore())
	dagStore, err := dagstore.NewDAGStore(dagstore.Config{
		TransientsDir: filepath.Join(dagStoreDir, "transients"),
		IndexDir:      filepath.Join(dagStoreDir, "index"),
		Datastore:     dagStoreDS,
	})
	require.NoError(t, err)
	consOpts := kit.ConstructorOpts(
		node.Override(new(*dagstore.DAGStore), dagStore),
	)

	// Set up the first provider
	opts := []kit.NodeOpt{kit.WithAllSubsystems(), consOpts}
	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&client, opts...).
		Miner(&provider1, &client, opts...).
		Start()

	ens.InterconnectAll().BeginMining(blockTime, &provider1)
	dh := kit.NewDealHarness(t, &client, &provider1, &provider1)

	// Make a storage deal against the first provider
	deal, res, inPath := dh.MakeOnlineDeal(context.Background(), kit.MakeFullDealParams{
		Rseed:      5,
		StartEpoch: startEpoch,
	})

	// Set up the second provider
	provider2Opts := append(opts,
		kit.OwnerAddr(client.DefaultKey),
		kit.MainMiner(&provider1),
	)
	ens.Miner(&provider2, &client, provider2Opts...).
		Start().
		Connect(provider2, client)

	// Make a retrieval deal against the second provider
	dh2 := kit.NewDealHarness(t, &client, &provider2, &provider2)
	outPath := dh2.PerformRetrieval(context.Background(), deal, res.Root, false)
	kit.AssertFilesEqual(t, inPath, outPath)
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
