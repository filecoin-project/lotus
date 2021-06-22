package itests

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/stretchr/testify/require"
)

func TestDealCycle(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	dealStartEpoch := abi.ChainEpoch(2 << 12)

	t.Run("TestFullDealCycle_Single", func(t *testing.T) {
		runFullDealCycles(t, 1, kit.MockMinerBuilder, blockTime, false, false, dealStartEpoch)
	})
	t.Run("TestFullDealCycle_Two", func(t *testing.T) {
		runFullDealCycles(t, 2, kit.MockMinerBuilder, blockTime, false, false, dealStartEpoch)
	})
	t.Run("WithExportedCAR", func(t *testing.T) {
		runFullDealCycles(t, 1, kit.MockMinerBuilder, blockTime, true, false, dealStartEpoch)
	})
	t.Run("TestFastRetrievalDealCycle", func(t *testing.T) {
		runFastRetrievalDealFlowT(t, kit.MockMinerBuilder, blockTime, dealStartEpoch)
	})
	t.Run("TestZeroPricePerByteRetrievalDealFlow", func(t *testing.T) {
		runZeroPricePerByteRetrievalDealFlow(t, kit.MockMinerBuilder, blockTime, dealStartEpoch)
	})
}

func TestAPIDealFlowReal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	// TODO: just set this globally?
	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	t.Run("basic", func(t *testing.T) {
		runFullDealCycles(t, 1, kit.Builder, time.Second, false, false, 0)
	})

	t.Run("fast-retrieval", func(t *testing.T) {
		runFullDealCycles(t, 1, kit.Builder, time.Second, false, true, 0)
	})

	t.Run("retrieval-second", func(t *testing.T) {
		runSecondDealRetrievalTest(t, kit.Builder, time.Second)
	})

	t.Run("quote-price-for-non-unsealed-retrieval", func(t *testing.T) {
		runQuotePriceForUnsealedRetrieval(t, kit.Builder, time.Second, 0)
	})
}

func runQuotePriceForUnsealedRetrieval(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	ctx := context.Background()
	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]
	kit.ConnectAndStartMining(t, blocktime, miner, client)

	ppb := int64(1)
	unsealPrice := int64(77)

	// Set unsealed price to non-zero
	ask, err := miner.MarketGetRetrievalAsk(ctx)
	require.NoError(t, err)
	ask.PricePerByte = abi.NewTokenAmount(ppb)
	ask.UnsealPrice = abi.NewTokenAmount(unsealPrice)
	err = miner.MarketSetRetrievalAsk(ctx, ask)
	require.NoError(t, err)

	dh := kit.NewDealHarness(t, client, miner)

	_, info, fcid := dh.MakeFullDeal(kit.MakeFullDealParams{
		Ctx:         ctx,
		Rseed:       6,
		CarExport:   false,
		FastRet:     false,
		StartEpoch:  startEpoch,
		DoRetrieval: false,
	})

	// one more storage deal for the same data
	_, _, fcid2 := dh.MakeFullDeal(kit.MakeFullDealParams{
		Ctx:         ctx,
		Rseed:       6,
		CarExport:   false,
		FastRet:     false,
		StartEpoch:  startEpoch,
		DoRetrieval: false,
	})
	require.Equal(t, fcid, fcid2)

	// fetch quote -> zero for unsealed price since unsealed file already exists.
	offers, err := client.ClientFindData(ctx, fcid, &info.PieceCID)
	require.NoError(t, err)
	require.Len(t, offers, 2)
	require.Equal(t, offers[0], offers[1])
	require.Equal(t, uint64(0), offers[0].UnsealPrice.Uint64())
	require.Equal(t, info.Size*uint64(ppb), offers[0].MinPrice.Uint64())

	// remove ONLY one unsealed file
	ss, err := miner.StorageList(context.Background())
	require.NoError(t, err)
	_, err = miner.SectorsList(ctx)
	require.NoError(t, err)

iLoop:
	for storeID, sd := range ss {
		for _, sector := range sd {
			require.NoError(t, miner.StorageDropSector(ctx, storeID, sector.SectorID, storiface.FTUnsealed))
			// remove ONLY one
			break iLoop
		}
	}

	// get retrieval quote -> zero for unsealed price as unsealed file exists.
	offers, err = client.ClientFindData(ctx, fcid, &info.PieceCID)
	require.NoError(t, err)
	require.Len(t, offers, 2)
	require.Equal(t, offers[0], offers[1])
	require.Equal(t, uint64(0), offers[0].UnsealPrice.Uint64())
	require.Equal(t, info.Size*uint64(ppb), offers[0].MinPrice.Uint64())

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
	offers, err = client.ClientFindData(ctx, fcid, &info.PieceCID)
	require.NoError(t, err)
	require.Len(t, offers, 2)
	require.Equal(t, offers[0], offers[1])
	require.Equal(t, uint64(unsealPrice), offers[0].UnsealPrice.Uint64())
	total := (info.Size * uint64(ppb)) + uint64(unsealPrice)
	require.Equal(t, total, offers[0].MinPrice.Uint64())
}

func TestPublishDealsBatching(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	b := kit.MockMinerBuilder
	blocktime := 10 * time.Millisecond
	startEpoch := abi.ChainEpoch(2 << 12)

	publishPeriod := 10 * time.Second
	maxDealsPerMsg := uint64(2)

	// Set max deals per publish deals message to 2
	minerDef := []kit.StorageMiner{{
		Full: 0,
		Opts: node.Override(
			new(*storageadapter.DealPublisher),
			storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
				Period:         publishPeriod,
				MaxDealsPerMsg: maxDealsPerMsg,
			})),
		Preseal: kit.PresealGenesis,
	}}

	// Create a connect client and miner node
	n, sn := b(t, kit.OneFull, minerDef)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	kit.ConnectAndStartMining(t, blocktime, miner, client)

	dh := kit.NewDealHarness(t, client, miner)

	// Starts a deal and waits until it's published
	runDealTillPublish := func(rseed int) {
		res, _, _, err := kit.CreateImportFile(ctx, client, rseed, 0)
		require.NoError(t, err)

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

func TestDealMining(t *testing.T) {
	// test making a deal with a fresh miner, and see if it starts to mine.
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	b := kit.MockMinerBuilder
	blocktime := 50 * time.Millisecond

	ctx := context.Background()
	fulls, miners := b(t,
		kit.OneFull,
		[]kit.StorageMiner{
			{Full: 0, Preseal: kit.PresealGenesis},
			{Full: 0, Preseal: 0}, // TODO: Add support for miners on non-first full node
		})
	client := fulls[0].FullNode.(*impl.FullNodeAPI)
	genesisMiner := miners[0]
	provider := miners[1]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := provider.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}

	if err := genesisMiner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	data := make([]byte, 600)
	rand.New(rand.NewSource(5)).Read(data)

	r := bytes.NewReader(data)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	var mine int32 = 1
	done := make(chan struct{})
	minedTwo := make(chan struct{})

	m2addr, err := miners[1].ActorAddress(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer close(done)

		complChan := minedTwo
		for atomic.LoadInt32(&mine) != 0 {
			wait := make(chan int)
			mdone := func(mined bool, _ abi.ChainEpoch, err error) {
				n := 0
				if mined {
					n = 1
				}
				wait <- n
			}

			if err := miners[0].MineOne(ctx, miner.MineReq{Done: mdone}); err != nil {
				t.Error(err)
			}

			if err := miners[1].MineOne(ctx, miner.MineReq{Done: mdone}); err != nil {
				t.Error(err)
			}

			expect := <-wait
			expect += <-wait

			time.Sleep(blocktime)
			if expect == 0 {
				// null block
				continue
			}

			var nodeOneMined bool
			for _, node := range miners {
				mb, err := node.MiningBase(ctx)
				if err != nil {
					t.Error(err)
					return
				}

				for _, b := range mb.Blocks() {
					if b.Miner == m2addr {
						nodeOneMined = true
						break
					}
				}

			}

			if nodeOneMined && complChan != nil {
				close(complChan)
				complChan = nil
			}

		}
	}()

	dh := kit.NewDealHarness(t, client, provider)

	deal := dh.StartDeal(ctx, fcid, false, 0)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	dh.WaitDealSealed(ctx, deal, false, false, nil)

	<-minedTwo

	atomic.StoreInt32(&mine, 0)
	fmt.Println("shutting down mining")
	<-done
}

func TestOfflineDealFlow(t *testing.T) {
	blocktime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T, fastRet bool) {
		ctx := context.Background()
		fulls, miners := kit.MockMinerBuilder(t, kit.OneFull, kit.OneMiner)
		client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]

		kit.ConnectAndStartMining(t, blocktime, miner, client)

		dh := kit.NewDealHarness(t, client, miner)

		// Create a random file and import on the client.
		res, path, data, err := kit.CreateImportFile(ctx, client, 1, 0)
		require.NoError(t, err)

		// Get the piece size and commP
		fcid := res.Root
		pieceInfo, err := client.ClientDealPieceCID(ctx, fcid)
		require.NoError(t, err)
		fmt.Println("FILE CID: ", fcid)

		// Create a storage deal with the miner
		maddr, err := miner.ActorAddress(ctx)
		require.NoError(t, err)

		addr, err := client.WalletDefaultAddress(ctx)
		require.NoError(t, err)

		// Manual storage deal (offline deal)
		dataRef := &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			Root:         fcid,
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
		carFileDir, err := ioutil.TempDir(os.TempDir(), "test-make-deal-car")
		require.NoError(t, err)
		carFilePath := filepath.Join(carFileDir, "out.car")
		err = client.ClientGenCar(ctx, api.FileRef{Path: path}, carFilePath)
		require.NoError(t, err)

		// Import the CAR file on the miner - this is the equivalent to
		// transferring the file across the wire in a normal (non-offline) deal
		err = miner.DealsImportData(ctx, *proposalCid, carFilePath)
		require.NoError(t, err)

		// Wait for the deal to be published
		dh.WaitDealPublished(ctx, proposalCid)

		t.Logf("deal published, retrieving")

		// Retrieve the deal
		dh.TestRetrieval(ctx, fcid, &pieceInfo.PieceCID, false, data)
	}

	t.Run("NormalRetrieval", func(t *testing.T) {
		runTest(t, false)
	})
	t.Run("FastRetrieval", func(t *testing.T) {
		runTest(t, true)
	})

}

func runFullDealCycles(t *testing.T, n int, b kit.APIBuilder, blocktime time.Duration, carExport, fastRet bool, startEpoch abi.ChainEpoch) {
	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]

	kit.ConnectAndStartMining(t, blocktime, miner, client)

	dh := kit.NewDealHarness(t, client, miner)

	baseseed := 6
	for i := 0; i < n; i++ {
		_, _, _ = dh.MakeFullDeal(kit.MakeFullDealParams{
			Ctx:         context.Background(),
			Rseed:       baseseed + i,
			CarExport:   carExport,
			FastRet:     fastRet,
			StartEpoch:  startEpoch,
			DoRetrieval: true,
		})
	}
}

func runFastRetrievalDealFlowT(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	ctx := context.Background()

	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]

	kit.ConnectAndStartMining(t, blocktime, miner, client)

	dh := kit.NewDealHarness(t, client, miner)

	data := make([]byte, 1600)
	rand.New(rand.NewSource(int64(8))).Read(data)

	r := bytes.NewReader(data)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	deal := dh.StartDeal(ctx, fcid, true, startEpoch)
	dh.WaitDealPublished(ctx, deal)

	fmt.Println("deal published, retrieving")

	// Retrieval
	info, err := client.ClientGetDealInfo(ctx, *deal)
	require.NoError(t, err)

	dh.TestRetrieval(ctx, fcid, &info.PieceCID, false, data)
}

func runSecondDealRetrievalTest(t *testing.T, b kit.APIBuilder, blocktime time.Duration) {
	ctx := context.Background()

	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]

	kit.ConnectAndStartMining(t, blocktime, miner, client)

	dh := kit.NewDealHarness(t, client, miner)

	{
		data1 := make([]byte, 800)
		rand.New(rand.NewSource(int64(3))).Read(data1)
		r := bytes.NewReader(data1)

		fcid1, err := client.ClientImportLocal(ctx, r)
		if err != nil {
			t.Fatal(err)
		}

		data2 := make([]byte, 800)
		rand.New(rand.NewSource(int64(9))).Read(data2)
		r2 := bytes.NewReader(data2)

		fcid2, err := client.ClientImportLocal(ctx, r2)
		if err != nil {
			t.Fatal(err)
		}

		deal1 := dh.StartDeal(ctx, fcid1, true, 0)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)
		dh.WaitDealSealed(ctx, deal1, true, false, nil)

		deal2 := dh.StartDeal(ctx, fcid2, true, 0)

		time.Sleep(time.Second)
		dh.WaitDealSealed(ctx, deal2, false, false, nil)

		// Retrieval
		info, err := client.ClientGetDealInfo(ctx, *deal2)
		require.NoError(t, err)

		rf, _ := miner.SectorsRefs(ctx)
		fmt.Printf("refs: %+v\n", rf)

		dh.TestRetrieval(ctx, fcid2, &info.PieceCID, false, data2)
	}
}

func runZeroPricePerByteRetrievalDealFlow(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	ctx := context.Background()

	fulls, miners := b(t, kit.OneFull, kit.OneMiner)
	client, miner := fulls[0].FullNode.(*impl.FullNodeAPI), miners[0]

	kit.ConnectAndStartMining(t, blocktime, miner, client)

	dh := kit.NewDealHarness(t, client, miner)

	// Set price-per-byte to zero
	ask, err := miner.MarketGetRetrievalAsk(ctx)
	require.NoError(t, err)

	ask.PricePerByte = abi.NewTokenAmount(0)
	err = miner.MarketSetRetrievalAsk(ctx, ask)
	require.NoError(t, err)

	_, _, _ = dh.MakeFullDeal(kit.MakeFullDealParams{
		Ctx:         ctx,
		Rseed:       6,
		CarExport:   false,
		FastRet:     false,
		StartEpoch:  startEpoch,
		DoRetrieval: true,
	})
}
