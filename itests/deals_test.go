package itests

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

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
		dh := kit.NewDealHarness(t, client, miner)

		runConcurrentDeals(t, dh, fullDealCyclesOpts{
			n:             n,
			fastRetrieval: fastRetrieval,
			carExport:     carExport,
			startEpoch:    startEpoch,
		})
	}

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

type fullDealCyclesOpts struct {
	n             int
	fastRetrieval bool
	carExport     bool
	startEpoch    abi.ChainEpoch
}

func runConcurrentDeals(t *testing.T, dh *kit.DealHarness, opts fullDealCyclesOpts) {
	errgrp, _ := errgroup.WithContext(context.Background())
	for i := 0; i < opts.n; i++ {
		i := i
		errgrp.Go(func() (err error) {
			defer func() {
				// This is necessary because golang can't deal with test
				// failures being reported from children goroutines ¯\_(ツ)_/¯
				if r := recover(); r != nil {
					err = fmt.Errorf("deal failed: %s", r)
				}
			}()
			deal, res, inPath := dh.MakeOnlineDeal(context.Background(), kit.MakeFullDealParams{
				Rseed:      5 + i,
				FastRet:    opts.fastRetrieval,
				StartEpoch: opts.startEpoch,
			})
			outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, opts.carExport)
			kit.AssertFilesEqual(t, inPath, outPath)
			return nil
		})
	}
	require.NoError(t, errgrp.Wait())
}

func TestDealsWithSealingAndRPC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	var blockTime = 1 * time.Second

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC()) // no mock proofs.
	ens.InterconnectAll().BeginMining(blockTime)
	dh := kit.NewDealHarness(t, client, miner)

	t.Run("stdretrieval", func(t *testing.T) {
		runConcurrentDeals(t, dh, fullDealCyclesOpts{n: 1})
	})

	t.Run("fastretrieval", func(t *testing.T) {
		runConcurrentDeals(t, dh, fullDealCyclesOpts{n: 1, fastRetrieval: true})
	})

	t.Run("fastretrieval-twodeals-sequential", func(t *testing.T) {
		runConcurrentDeals(t, dh, fullDealCyclesOpts{n: 1, fastRetrieval: true})
		runConcurrentDeals(t, dh, fullDealCyclesOpts{n: 1, fastRetrieval: true})
	})
}

func TestQuotePriceForUnsealedRetrieval(t *testing.T) {
	var (
		ctx       = context.Background()
		blocktime = 10 * time.Millisecond
	)

	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
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

	dh := kit.NewDealHarness(t, client, miner)

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

func TestPublishDealsBatching(t *testing.T) {
	var (
		ctx            = context.Background()
		publishPeriod  = 10 * time.Second
		maxDealsPerMsg = uint64(2) // Set max deals per publish deals message to 2
		startEpoch     = abi.ChainEpoch(2 << 12)
	)

	kit.QuietMiningLogs()

	opts := node.Override(new(*storageadapter.DealPublisher),
		storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
			Period:         publishPeriod,
			MaxDealsPerMsg: maxDealsPerMsg,
		}),
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ConstructorOpts(opts))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	dh := kit.NewDealHarness(t, client, miner)

	// Starts a deal and waits until it's published
	runDealTillPublish := func(rseed int) {
		res, _ := client.CreateImportFile(ctx, rseed, 0)

		upds, err := client.ClientGetDealUpdates(ctx)
		require.NoError(t, err)

		dh.StartDeal(ctx, res.Root, false, startEpoch)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)

		done := make(chan struct{})
		go func() {
			for upd := range upds {
				if upd.DataRef.Root == res.Root && upd.State == storagemarket.StorageDealAwaitingPreCommit {
					done <- struct{}{}
				}
			}
		}()
		<-done
	}

	// Run three deals in parallel
	done := make(chan struct{}, maxDealsPerMsg+1)
	for rseed := 1; rseed <= 3; rseed++ {
		rseed := rseed
		go func() {
			runDealTillPublish(rseed)
			done <- struct{}{}
		}()
	}

	// Wait for two of the deals to be published
	for i := 0; i < int(maxDealsPerMsg); i++ {
		<-done
	}

	// Expect a single PublishStorageDeals message that includes the first two deals
	msgCids, err := client.StateListMessages(ctx, &api.MessageMatch{To: market.Address}, types.EmptyTSK, 1)
	require.NoError(t, err)
	count := 0
	for _, msgCid := range msgCids {
		msg, err := client.ChainGetMessage(ctx, msgCid)
		require.NoError(t, err)

		if msg.Method == market.Methods.PublishStorageDeals {
			count++
			var pubDealsParams market2.PublishStorageDealsParams
			err = pubDealsParams.UnmarshalCBOR(bytes.NewReader(msg.Params))
			require.NoError(t, err)
			require.Len(t, pubDealsParams.Deals, int(maxDealsPerMsg))
		}
	}
	require.Equal(t, 1, count)

	// The third deal should be published once the publish period expires.
	// Allow a little padding as it takes a moment for the state change to
	// be noticed by the client.
	padding := 10 * time.Second
	select {
	case <-time.After(publishPeriod + padding):
		require.Fail(t, "Expected 3rd deal to be published once publish period elapsed")
	case <-done: // Success
	}
}

func TestFirstDealEnablesMining(t *testing.T) {
	// test making a deal with a fresh miner, and see if it starts to mine.
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	var (
		client   kit.TestFullNode
		genMiner kit.TestMiner // bootstrap
		provider kit.TestMiner // no sectors, will need to create one
	)

	ens := kit.NewEnsemble(t, kit.MockProofs())
	ens.FullNode(&client)
	ens.Miner(&genMiner, &client)
	ens.Miner(&provider, &client, kit.PresealSectors(0))
	ens.Start().InterconnectAll().BeginMining(50 * time.Millisecond)

	ctx := context.Background()

	dh := kit.NewDealHarness(t, &client, &provider)

	ref, _ := client.CreateImportFile(ctx, 5, 0)

	t.Log("FILE CID:", ref.Root)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// start a goroutine to monitor head changes from the client
	// once the provider has mined a block, thanks to the power acquired from the deal,
	// we pass the test.
	providerMined := make(chan struct{})

	go func() {
		_ = client.WaitTillChain(ctx, kit.BlockMinedBy(provider.ActorAddr))
		close(providerMined)
	}()

	// now perform the deal.
	deal := dh.StartDeal(ctx, ref.Root, false, 0)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	dh.WaitDealSealed(ctx, deal, false, false, nil)

	<-providerMined
}

func TestOfflineDealFlow(t *testing.T) {
	blocktime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T, fastRet bool) {
		ctx := context.Background()
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
		ens.InterconnectAll().BeginMining(blocktime)

		dh := kit.NewDealHarness(t, client, miner)

		// Create a random file and import on the client.
		res, inFile := client.CreateImportFile(ctx, 1, 0)

		// Get the piece size and commP
		rootCid := res.Root
		pieceInfo, err := client.ClientDealPieceCID(ctx, rootCid)
		require.NoError(t, err)
		t.Log("FILE CID:", rootCid)

		// Create a storage deal with the miner
		maddr, err := miner.ActorAddress(ctx)
		require.NoError(t, err)

		addr, err := client.WalletDefaultAddress(ctx)
		require.NoError(t, err)

		// Manual storage deal (offline deal)
		dataRef := &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			Root:         rootCid,
			PieceCid:     &pieceInfo.PieceCID,
			PieceSize:    pieceInfo.PieceSize.Unpadded(),
		}

		proposalCid, err := client.ClientStartDeal(ctx, &api.StartDealParams{
			Data:              dataRef,
			Wallet:            addr,
			Miner:             maddr,
			EpochPrice:        types.NewInt(1000000),
			DealStartEpoch:    startEpoch,
			MinBlocksDuration: uint64(build.MinDealDuration),
			FastRetrieval:     fastRet,
		})
		require.NoError(t, err)

		// Wait for the deal to reach StorageDealCheckForAcceptance on the client
		cd, err := client.ClientGetDealInfo(ctx, *proposalCid)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			cd, _ := client.ClientGetDealInfo(ctx, *proposalCid)
			return cd.State == storagemarket.StorageDealCheckForAcceptance
		}, 30*time.Second, 1*time.Second, "actual deal status is %s", storagemarket.DealStates[cd.State])

		// Create a CAR file from the raw file
		carFileDir := t.TempDir()
		carFilePath := filepath.Join(carFileDir, "out.car")
		err = client.ClientGenCar(ctx, api.FileRef{Path: inFile}, carFilePath)
		require.NoError(t, err)

		// Import the CAR file on the miner - this is the equivalent to
		// transferring the file across the wire in a normal (non-offline) deal
		err = miner.DealsImportData(ctx, *proposalCid, carFilePath)
		require.NoError(t, err)

		// Wait for the deal to be published
		dh.WaitDealPublished(ctx, proposalCid)

		t.Logf("deal published, retrieving")

		// Retrieve the deal
		outFile := dh.PerformRetrieval(ctx, proposalCid, rootCid, false)

		kit.AssertFilesEqual(t, inFile, outFile)

	}

	t.Run("stdretrieval", func(t *testing.T) { runTest(t, false) })
	t.Run("fastretrieval", func(t *testing.T) { runTest(t, true) })
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

	dh := kit.NewDealHarness(t, client, miner)
	runConcurrentDeals(t, dh, fullDealCyclesOpts{
		n:          1,
		startEpoch: startEpoch,
	})
}
