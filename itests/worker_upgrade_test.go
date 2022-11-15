package itests

import (
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestWorkerUpgradeAbortCleanup(t *testing.T) {
	ctx := context.Background()
	blockTime := 1 * time.Millisecond
	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithNoLocalSealing(true),
		kit.NoStorage(), // no storage to have better control over path settings
		kit.MutateSealingConfig(func(sc *config.SealingConfig) { sc.FinalizeEarly = true })) // no mock proofs

	var worker kit.TestWorker
	ens.Worker(miner, &worker, kit.ThroughRPC(), kit.NoStorage(), // no storage to have better control over path settings
		kit.WithTaskTypes([]sealtasks.TaskType{
			sealtasks.TTFetch, sealtasks.TTAddPiece,
			sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2,
			sealtasks.TTReplicaUpdate, // only first update step, later steps will not run and we'll abort
		}),
	)

	ens.Start().InterconnectAll().BeginMiningMustPost(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// get storage paths

	// store-only path on the miner
	miner.AddStorage(ctx, t, func(cfg *storiface.LocalStorageMeta) {
		cfg.CanSeal = false
		cfg.CanStore = true
	})

	mlocal, err := miner.StorageLocal(ctx)
	require.NoError(t, err)
	require.Len(t, mlocal, 2) // genesis and one local

	// we want a seal-only path on the worker disconnected from miner path
	worker.AddStorage(ctx, t, func(cfg *storiface.LocalStorageMeta) {
		cfg.CanSeal = true
		cfg.CanStore = false
	})

	wpaths, err := worker.Paths(ctx)
	require.NoError(t, err)
	require.Len(t, wpaths, 1)

	// check sectors in paths
	checkSectors := func(miners, workers storiface.SectorFileType) {
		paths, err := miner.StorageList(ctx)
		require.NoError(t, err)
		require.Len(t, paths, 3) // genesis, miner, worker

		// first loop for debugging
		for id, decls := range paths {
			pinfo, err := miner.StorageInfo(ctx, id)
			require.NoError(t, err)

			switch {
			case id == wpaths[0].ID: // worker path
				fmt.Println("Worker Decls ", len(decls), decls)
			case !pinfo.CanStore && !pinfo.CanSeal: // genesis path
				fmt.Println("Genesis Decls ", len(decls), decls)
			default: // miner path
				fmt.Println("Miner Decls ", len(decls), decls)
			}
		}

		for id, decls := range paths {
			pinfo, err := miner.StorageInfo(ctx, id)
			require.NoError(t, err)

			switch {
			case id == wpaths[0].ID: // worker path
				if workers != storiface.FTNone {
					require.Len(t, decls, 1)
					require.EqualValues(t, decls[0].SectorFileType.Strings(), workers.Strings())
				} else {
					require.Len(t, decls, 0)
				}
			case !pinfo.CanStore && !pinfo.CanSeal: // genesis path
				require.Len(t, decls, kit.DefaultPresealsPerBootstrapMiner)
			default: // miner path
				if miners != storiface.FTNone {
					require.Len(t, decls, 1)
					require.EqualValues(t, decls[0].SectorFileType.Strings(), miners.Strings())
				} else {
					require.Len(t, decls, 0)
				}
			}
		}
	}
	checkSectors(storiface.FTNone, storiface.FTNone)

	// get a sector for upgrading
	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	snum := sl[0]

	checkSectors(storiface.FTCache|storiface.FTSealed, storiface.FTNone)

	client.WaitForSectorActive(ctx, t, snum, maddr)

	// make available
	err = miner.SectorMarkForUpgrade(ctx, snum, true)
	require.NoError(t, err)

	// Start a deal

	dh := kit.NewDealHarness(t, client, miner, miner)
	res, _ := client.CreateImportFile(ctx, 123, 0)
	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	deal := dh.StartDeal(ctx, dp)

	// wait for the deal to be in a sector
	dh.WaitDealSealed(ctx, deal, true, false, nil)

	// wait for replica update to happen
	require.Eventually(t, func() bool {
		sstate, err := miner.SectorsStatus(ctx, snum, false)
		require.NoError(t, err)
		return sstate.State == api.SectorState(sealing.ProveReplicaUpdate)
	}, 10*time.Second, 50*time.Millisecond)

	// check that the sector was copied to the worker
	checkSectors(storiface.FTCache|storiface.FTSealed, storiface.FTCache|storiface.FTSealed|storiface.FTUnsealed|storiface.FTUpdate|storiface.FTUpdateCache)

	// abort upgrade
	err = miner.SectorAbortUpgrade(ctx, snum)
	require.NoError(t, err)

	// the task is stuck in scheduler, so manually abort the task to get the sector fsm moving
	si := miner.SchedInfo(ctx)
	err = miner.SealingRemoveRequest(ctx, si.SchedInfo.Requests[0].SchedId)
	require.NoError(t, err)

	var lastState api.SectorState
	require.Eventually(t, func() bool {
		sstate, err := miner.SectorsStatus(ctx, snum, false)
		require.NoError(t, err)
		lastState = sstate.State

		return sstate.State == api.SectorState(sealing.Proving)
	}, 10*time.Second, 50*time.Millisecond, "last state was %s", &lastState)

	// check that nothing was left on the worker
	checkSectors(storiface.FTCache|storiface.FTSealed, storiface.FTNone)
}
