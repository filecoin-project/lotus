package itests

import (
	"context"
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
			sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2,
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

	// get a sector for upgrading
	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	sector := sl[0]

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
}
