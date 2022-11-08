package itests

import (
	"context"
	"strings"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestPathTypeFilters(t *testing.T) {
	runTest := func(t *testing.T, name string, asserts func(t *testing.T, ctx context.Context, miner *kit.TestMiner, run func())) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			_ = logging.SetLogLevel("storageminer", "INFO")

			var (
				client   kit.TestFullNode
				miner    kit.TestMiner
				wiw, wdw kit.TestWorker
			)
			ens := kit.NewEnsemble(t, kit.LatestActorsAt(-1)).
				FullNode(&client, kit.ThroughRPC()).
				Miner(&miner, &client, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.PresealSectors(2), kit.NoStorage()).
				Worker(&miner, &wiw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWinningPoSt})).
				Worker(&miner, &wdw, kit.ThroughRPC(), kit.NoStorage(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt})).
				Start()

			ens.InterconnectAll().BeginMiningMustPost(2 * time.Millisecond)

			asserts(t, ctx, &miner, func() {
				dh := kit.NewDealHarness(t, &client, &miner, &miner)
				dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1})
			})
		})
	}

	runTest(t, "invalid-type-alert", func(t *testing.T, ctx context.Context, miner *kit.TestMiner, run func()) {
		slU := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanSeal = true
			meta.AllowTypes = []string{"unsealed", "seeled"}
		})

		storlist, err := miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist, 2) // 1 path we've added + preseal

		si, err := miner.StorageInfo(ctx, slU)
		require.NoError(t, err)

		// check that bad entries are filtered
		require.Len(t, si.DenyTypes, 0)
		require.Len(t, si.AllowTypes, 1)
		require.Equal(t, "unsealed", si.AllowTypes[0])

		as, err := miner.LogAlerts(ctx)
		require.NoError(t, err)

		var found bool
		for _, a := range as {
			if a.Active && a.Type.System == "sector-index" && strings.HasPrefix(a.Type.Subsystem, "pathconf-") {
				require.False(t, found)
				require.Contains(t, string(a.LastActive.Message), "unknown sector file type 'seeled'")
				found = true
			}
		}
		require.True(t, found)
	})

	runTest(t, "seal-to-stor-unseal-allowdeny", func(t *testing.T, ctx context.Context, miner *kit.TestMiner, run func()) {
		// allow all types in the sealing path
		sealScratch := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanSeal = true
		})

		// unsealed storage
		unsStor := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.AllowTypes = []string{"unsealed"}
		})

		// other storage
		sealStor := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.DenyTypes = []string{"unsealed"}
		})

		storlist, err := miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist, 4) // 3 paths we've added + preseal

		run()

		storlist, err = miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist[sealScratch], 0)
		require.Len(t, storlist[unsStor], 1)
		require.Len(t, storlist[sealStor], 1)

		require.Equal(t, storiface.FTUnsealed, storlist[unsStor][0].SectorFileType)
		require.Equal(t, storiface.FTSealed|storiface.FTCache, storlist[sealStor][0].SectorFileType)
	})

	runTest(t, "sealstor-unseal-allowdeny", func(t *testing.T, ctx context.Context, miner *kit.TestMiner, run func()) {
		// unsealed storage
		unsStor := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.CanSeal = true
			meta.AllowTypes = []string{"unsealed"}
		})

		// other storage
		sealStor := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.CanSeal = true
			meta.DenyTypes = []string{"unsealed"}
		})

		storlist, err := miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist, 3) // 2 paths we've added + preseal

		run()

		storlist, err = miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist[unsStor], 1)
		require.Len(t, storlist[sealStor], 1)

		require.Equal(t, storiface.FTUnsealed, storlist[unsStor][0].SectorFileType)
		require.Equal(t, storiface.FTSealed|storiface.FTCache, storlist[sealStor][0].SectorFileType)
	})

	runTest(t, "seal-store-allseparate", func(t *testing.T, ctx context.Context, miner *kit.TestMiner, run func()) {
		// sealing stores
		slU := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanSeal = true
			meta.AllowTypes = []string{"unsealed"}
		})
		slS := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanSeal = true
			meta.AllowTypes = []string{"sealed"}
		})
		slC := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanSeal = true
			meta.AllowTypes = []string{"cache"}
		})

		// storage stores
		stU := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.AllowTypes = []string{"unsealed"}
		})
		stS := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.AllowTypes = []string{"sealed"}
		})
		stC := miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
			meta.CanStore = true
			meta.AllowTypes = []string{"cache"}
		})

		storlist, err := miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist, 7) // 6 paths we've added + preseal

		run()

		storlist, err = miner.StorageList(ctx)
		require.NoError(t, err)

		require.Len(t, storlist[slU], 0)
		require.Len(t, storlist[slS], 0)
		require.Len(t, storlist[slC], 0)

		require.Len(t, storlist[stU], 1)
		require.Len(t, storlist[stS], 1)
		require.Len(t, storlist[stC], 1)

		require.Equal(t, storiface.FTUnsealed, storlist[stU][0].SectorFileType)
		require.Equal(t, storiface.FTSealed, storlist[stS][0].SectorFileType)
		require.Equal(t, storiface.FTCache, storlist[stC][0].SectorFileType)
	})
}
