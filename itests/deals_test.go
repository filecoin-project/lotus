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
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestDealCyclesConcurrent(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	dealStartEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T, n int, fastRetrieval bool, carExport bool) {
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
		ens.InterconnectAll().BeginMining(blockTime)
		dh := kit.NewDealHarness(t, client, miner)

		errgrp, _ := errgroup.WithContext(context.Background())
		for i := 0; i < n; i++ {
			i := i
			errgrp.Go(func() (err error) {
				defer func() {
					// This is necessary because we use require, which invokes t.Fatal,
					// and that's not
					if r := recover(); r != nil {
						err = fmt.Errorf("deal failed: %s", r)
					}
				}()
				deal, res, inPath := dh.MakeOnlineDeal(context.Background(), 5+i, fastRetrieval, dealStartEpoch)
				outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, carExport)
				kit.FilesEqual(t, inPath, outPath)
				return nil
			})
		}
		require.NoError(t, errgrp.Wait())
	}

	cycles := []int{1, 2, 4, 8}
	for _, n := range cycles {
		ns := fmt.Sprintf("%d", n)
		t.Run(ns+"-fastretrieval-CAR", func(t *testing.T) { runTest(t, n, true, true) })
		t.Run(ns+"-fastretrieval-NoCAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-CAR", func(t *testing.T) { runTest(t, n, true, false) })
		t.Run(ns+"-stdretrieval-NoCAR", func(t *testing.T) { runTest(t, n, false, false) })
	}
}

// func TestAPIDealFlowReal(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("skipping test in short mode")
// 	}
//
// 	kit.QuietMiningLogs()
//
// 	// TODO: just set this globally?
// 	oldDelay := policy.GetPreCommitChallengeDelay()
// 	policy.SetPreCommitChallengeDelay(5)
// 	t.Cleanup(func() {
// 		policy.SetPreCommitChallengeDelay(oldDelay)
// 	})
//
// 	t.Run("basic", func(t *testing.T) {
// 		runFullDealCycles(t, 1, kit.FullNodeBuilder, time.Second, false, false, 0)
// 	})
//
// 	t.Run("fast-retrieval", func(t *testing.T) {
// 		runFullDealCycles(t, 1, kit.FullNodeBuilder, time.Second, false, true, 0)
// 	})
//
// 	t.Run("retrieval-second", func(t *testing.T) {
// 		runSecondDealRetrievalTest(t, kit.FullNodeBuilder, time.Second)
// 	})
// }

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

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ExtraNodeOpts(opts))
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

	ens := kit.NewEnsemble(t)
	ens.FullNode(&client, kit.MockProofs())
	ens.Miner(&genMiner, &client, kit.MockProofs())
	ens.Miner(&provider, &client, kit.MockProofs(), kit.PresealSectors(0))
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
	heads, err := client.ChainNotify(ctx)
	require.NoError(t, err)

	go func() {
		for chg := range heads {
			for _, c := range chg {
				if c.Type != "apply" {
					continue
				}
				for _, b := range c.Val.Blocks() {
					if b.Miner == provider.ActorAddr {
						close(providerMined)
						return
					}
				}
			}
		}
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

		equal := kit.FilesEqual(t, inFile, outFile)
		require.True(t, equal)
	}

	t.Run("NormalRetrieval", func(t *testing.T) { runTest(t, false) })
	t.Run("FastRetrieval", func(t *testing.T) { runTest(t, true) })
}

//
// func runFastRetrievalDealFlowT(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
// 	ctx := context.Background()
//
// 	var (
// 		nb    = kit.NewNodeBuilder(t)
// 		full  = nb.FullNode()
// 		miner = nb.Miner(full)
// 	)
//
// 	nb.Create()
//
// 	kit.ConnectAndStartMining(t, blocktime, miner, full)
//
// 	dh := kit.NewDealHarness(t, full, miner)
// 	data := make([]byte, 1600)
// 	rand.New(rand.NewSource(int64(8))).Read(data)
//
// 	r := bytes.NewReader(data)
// 	fcid, err := full.FullNode.(*impl.FullNodeAPI).ClientImportLocal(ctx, r)
// 	require.NoError(t, err)
//
// 	fmt.Println("FILE CID: ", fcid)
//
// 	deal := dh.StartDeal(ctx, fcid, true, startEpoch)
// 	dh.WaitDealPublished(ctx, deal)
//
// 	fmt.Println("deal published, retrieving")
//
// 	// Retrieval
// 	info, err := full.ClientGetDealInfo(ctx, *deal)
// 	require.NoError(t, err)
//
// 	dh.PerformRetrieval(ctx, fcid, &info.PieceCID, false, data)
// }
//
// func runSecondDealRetrievalTest(t *testing.T, b kit.APIBuilder, blocktime time.Duration) {
// 	ctx := context.Background()
//
// 	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
// 	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]
//
// 	kit.ConnectAndStartMining(t, blocktime, miner, client)
//
// 	dh := kit.NewDealHarness(t, client, miner)
//
// 	{
// 		data1 := make([]byte, 800)
// 		rand.New(rand.NewSource(int64(3))).Read(data1)
// 		r := bytes.NewReader(data1)
//
// 		fcid1, err := client.ClientImportLocal(ctx, r)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
//
// 		data2 := make([]byte, 800)
// 		rand.New(rand.NewSource(int64(9))).Read(data2)
// 		r2 := bytes.NewReader(data2)
//
// 		fcid2, err := client.ClientImportLocal(ctx, r2)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
//
// 		deal1 := dh.StartDeal(ctx, fcid1, true, 0)
//
// 		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
// 		time.Sleep(time.Second)
// 		dh.WaitDealSealed(ctx, deal1, true, false, nil)
//
// 		deal2 := dh.StartDeal(ctx, fcid2, true, 0)
//
// 		time.Sleep(time.Second)
// 		dh.WaitDealSealed(ctx, deal2, false, false, nil)
//
// 		// Retrieval
// 		info, err := client.ClientGetDealInfo(ctx, *deal2)
// 		require.NoError(t, err)
//
// 		rf, _ := miner.SectorsRefs(ctx)
// 		fmt.Printf("refs: %+v\n", rf)
//
// 		dh.PerformRetrieval(ctx, fcid2, &info.PieceCID, false, data2)
// 	}
// }
//
// func runZeroPricePerByteRetrievalDealFlow(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
// 	ctx := context.Background()
//
// 	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
// 	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]
//
// 	kit.ConnectAndStartMining(t, blocktime, miner, client)
//
// 	dh := kit.NewDealHarness(t, client, miner)
//
// 	// Set price-per-byte to zero
// 	ask, err := miner.MarketGetRetrievalAsk(ctx)
// 	require.NoError(t, err)
//
// 	ask.PricePerByte = abi.NewTokenAmount(0)
// 	err = miner.MarketSetRetrievalAsk(ctx, ask)
// 	require.NoError(t, err)
//
// 	dh.MakeOnlineDeal(ctx, 6, false, false, startEpoch)
// }
