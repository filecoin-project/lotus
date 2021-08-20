package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestStorageDealMissingBlock(t *testing.T) {
	ctx := context.Background()

	// enable 512MiB proofs so we can conduct larger transfers.
	kit.EnableLargeSectors(t)
	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.SectorSize(512<<20), // 512MiB sectors.
	)
	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	dh := kit.NewDealHarness(t, client, miner, miner)

	client.WaitTillChain(ctx, kit.HeightAtLeast(5))

	res, _ := client.CreateImportFile(ctx, 0, 64<<20) // 64MiB file.
	list, err := client.ClientListImports(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, res.Root, *list[0].Root)

	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	dp.FastRetrieval = true
	dp.EpochPrice = abi.NewTokenAmount(62500000) // minimum asking price.
	deal := dh.StartDeal(ctx, dp)

	dh.WaitDealSealed(ctx, deal, false, false, nil)
}
