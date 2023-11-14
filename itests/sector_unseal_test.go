package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestUnsealPiece(t *testing.T) {
	ctx := context.Background()
	blockTime := 1 * time.Millisecond
	kit.QuietMiningLogs()

	_, miner, ens := kit.EnsembleMinimal(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithNoLocalSealing(true),
		kit.NoStorage(), // no storage to have better control over path settings
		kit.MutateSealingConfig(func(sc *config.SealingConfig) {
			sc.FinalizeEarly = true
			sc.AlwaysKeepUnsealedCopy = false
		})) // no mock proofs

	var worker kit.TestWorker
	ens.Worker(miner, &worker, kit.ThroughRPC(), kit.NoStorage(), // no storage to have better control over path settings
		kit.WithTaskTypes([]sealtasks.TaskType{
			sealtasks.TTFetch, sealtasks.TTAddPiece,
			sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2,
			sealtasks.TTReplicaUpdate, sealtasks.TTUnseal, // only first update step, later steps will not run and we'll abort
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

	// check which sectors files are present on the miner/worker storage paths
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
					require.EqualValues(t, workers.Strings(), decls[0].SectorFileType.Strings())
				} else {
					require.Len(t, decls, 0)
				}
			case !pinfo.CanStore && !pinfo.CanSeal: // genesis path
				require.Len(t, decls, kit.DefaultPresealsPerBootstrapMiner)
			default: // miner path
				if miners != storiface.FTNone {
					require.Len(t, decls, 1)
					require.EqualValues(t, miners.Strings(), decls[0].SectorFileType.Strings())
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

	sector := sl[0]

	checkSectors(storiface.FTCache|storiface.FTSealed, storiface.FTNone)

	sinfo, err := miner.SectorsStatus(ctx, sector, false)
	require.NoError(t, err)

	minerId, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	sectorRef := storiface.SectorRef{
		ID:        abi.SectorID{Miner: abi.ActorID(minerId), Number: sector},
		ProofType: sinfo.SealProof,
	}

	err = miner.SectorsUnsealPiece(ctx, sectorRef, 0, 0, sinfo.Ticket.Value, sinfo.CommD)
	require.NoError(t, err)

	checkSectors(storiface.FTCache|storiface.FTSealed|storiface.FTUnsealed, storiface.FTNone)
}
