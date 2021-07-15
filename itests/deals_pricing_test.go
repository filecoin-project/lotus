package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

func TestQuotePriceForUnsealedRetrieval(t *testing.T) {
	var (
		ctx       = context.Background()
		blocktime = 50 * time.Millisecond
	)

	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t)
	ens.InterconnectAll().BeginMining(blocktime)

	var (
		ppb         = int64(1)
		unsealPrice = int64(77)
	)

	// Set unsealed price to non-zero
	ask, err := miner.MarketGetRetrievalAsk(ctx)
	require.NoError(t, err)
	ask.PricePerByte = abi.NewTokenAmount(ppb)
	ask.UnsealPrice = abi.NewTokenAmount(unsealPrice)
	err = miner.MarketSetRetrievalAsk(ctx, ask)
	require.NoError(t, err)

	dh := kit.NewDealHarness(t, client, miner, miner)

	deal1, res1, _ := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{Rseed: 6})

	// one more storage deal for the same data
	_, res2, _ := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{Rseed: 6})
	require.Equal(t, res1.Root, res2.Root)

	// Retrieval
	dealInfo, err := client.ClientGetDealInfo(ctx, *deal1)
	require.NoError(t, err)

	// fetch quote -> zero for unsealed price since unsealed file already exists.
	offers, err := client.ClientFindData(ctx, res1.Root, &dealInfo.PieceCID)
	require.NoError(t, err)
	require.Len(t, offers, 2)
	require.Equal(t, offers[0], offers[1])
	require.Equal(t, uint64(0), offers[0].UnsealPrice.Uint64())
	require.Equal(t, dealInfo.Size*uint64(ppb), offers[0].MinPrice.Uint64())

	// remove ONLY one unsealed file
	ss, err := miner.StorageList(context.Background())
	require.NoError(t, err)
	_, err = miner.SectorsList(ctx)
	require.NoError(t, err)

iLoop:
	for storeID, sd := range ss {
		for _, sector := range sd {
			err := miner.StorageDropSector(ctx, storeID, sector.SectorID, storiface.FTUnsealed)
			require.NoError(t, err)
			break iLoop // remove ONLY one
		}
	}

	// get retrieval quote -> zero for unsealed price as unsealed file exists.
	offers, err = client.ClientFindData(ctx, res1.Root, &dealInfo.PieceCID)
	require.NoError(t, err)
	require.Len(t, offers, 2)
	require.Equal(t, offers[0], offers[1])
	require.Equal(t, uint64(0), offers[0].UnsealPrice.Uint64())
	require.Equal(t, dealInfo.Size*uint64(ppb), offers[0].MinPrice.Uint64())

	// remove the other unsealed file as well
	ss, err = miner.StorageList(context.Background())
	require.NoError(t, err)
	_, err = miner.SectorsList(ctx)
	require.NoError(t, err)
	for storeID, sd := range ss {
		for _, sector := range sd {
			require.NoError(t, miner.StorageDropSector(ctx, storeID, sector.SectorID, storiface.FTUnsealed))
		}
	}

	// fetch quote -> non-zero for unseal price as we no more unsealed files.
	offers, err = client.ClientFindData(ctx, res1.Root, &dealInfo.PieceCID)
	require.NoError(t, err)
	require.Len(t, offers, 2)
	require.Equal(t, offers[0], offers[1])
	require.Equal(t, uint64(unsealPrice), offers[0].UnsealPrice.Uint64())
	total := (dealInfo.Size * uint64(ppb)) + uint64(unsealPrice)
	require.Equal(t, total, offers[0].MinPrice.Uint64())
}

func TestZeroPricePerByteRetrieval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	var (
		blockTime  = 10 * time.Millisecond
		startEpoch = abi.ChainEpoch(2 << 12)
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx := context.Background()

	ask, err := miner.MarketGetRetrievalAsk(ctx)
	require.NoError(t, err)

	ask.PricePerByte = abi.NewTokenAmount(0)
	err = miner.MarketSetRetrievalAsk(ctx, ask)
	require.NoError(t, err)

	dh := kit.NewDealHarness(t, client, miner, miner)
	dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{
		N:          1,
		StartEpoch: startEpoch,
	})
}
