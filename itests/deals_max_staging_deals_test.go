package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestMaxStagingDeals(t *testing.T) {
	ctx := context.Background()

	// enable 512MiB proofs so we can conduct larger transfers.
	kit.EnableLargeSectors(t)
	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.WithMaxStagingDealsBytes(8192), // max 8KB staging deals
		kit.SectorSize(512<<20),            // 512MiB sectors.
	)
	ens.InterconnectAll().BeginMining(200 * time.Millisecond)

	dh := kit.NewDealHarness(t, client, miner, miner)

	client.WaitTillChain(ctx, kit.HeightAtLeast(5))

	res, _ := client.CreateImportFile(ctx, 0, 8192) // 8KB file
	list, err := client.ClientListImports(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	res2, _ := client.CreateImportFile(ctx, 0, 4096)
	list, err = client.ClientListImports(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// first deal stays in staging area, and is not yet passed to the sealing subsystem
	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	dp.FastRetrieval = true
	dp.EpochPrice = abi.NewTokenAmount(62500000) // minimum asking price.
	deal := dh.StartDeal(ctx, dp)

	time.Sleep(1 * time.Second)

	// expecting second deal to fail since staging area is full
	dp.Data.Root = res2.Root
	dp.FastRetrieval = true
	dp.EpochPrice = abi.NewTokenAmount(62500000) // minimum asking price.
	deal2 := dh.StartDeal(ctx, dp)

	_ = deal

	err = dh.ExpectDealFailure(ctx, deal2, "cannot accept deal as miner is overloaded at the moment")
	if err != nil {
		t.Fatal(err)
	}
}
