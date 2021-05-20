package itests

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
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
}

func TestPublishDealsBatching(t *testing.T) {
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
	s := kit.ConnectAndStartMining(t, blocktime, client, miner)
	defer s.BlockMiner.Stop()

	// Starts a deal and waits until it's published
	runDealTillPublish := func(rseed int) {
		res, _, err := kit.CreateClientFile(s.Ctx, s.Client, rseed)
		require.NoError(t, err)

		upds, err := client.ClientGetDealUpdates(s.Ctx)
		require.NoError(t, err)

		kit.StartDeal(t, s.Ctx, s.Miner, s.Client, res.Root, false, startEpoch)

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
	msgCids, err := s.Client.StateListMessages(s.Ctx, &api.MessageMatch{To: market.Address}, types.EmptyTSK, 1)
	require.NoError(t, err)
	count := 0
	for _, msgCid := range msgCids {
		msg, err := s.Client.ChainGetMessage(s.Ctx, msgCid)
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
	n, sn := b(t, kit.OneFull, []kit.StorageMiner{
		{Full: 0, Preseal: kit.PresealGenesis},
		{Full: 0, Preseal: 0}, // TODO: Add support for miners on non-first full node
	})
	client := n[0].FullNode.(*impl.FullNodeAPI)
	provider := sn[1]
	genesisMiner := sn[0]

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

	m2addr, err := sn[1].ActorAddress(context.TODO())
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

			if err := sn[0].MineOne(ctx, miner.MineReq{Done: mdone}); err != nil {
				t.Error(err)
			}

			if err := sn[1].MineOne(ctx, miner.MineReq{Done: mdone}); err != nil {
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
			for _, node := range sn {
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

	deal := kit.StartDeal(t, ctx, provider, client, fcid, false, 0)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	kit.WaitDealSealed(t, ctx, provider, client, deal, false)

	<-minedTwo

	atomic.StoreInt32(&mine, 0)
	fmt.Println("shutting down mining")
	<-done
}

func runFullDealCycles(t *testing.T, n int, b kit.APIBuilder, blocktime time.Duration, carExport, fastRet bool, startEpoch abi.ChainEpoch) {
	s := kit.SetupOneClientOneMiner(t, b, blocktime)
	defer s.BlockMiner.Stop()

	baseseed := 6
	for i := 0; i < n; i++ {
		kit.MakeDeal(t, s.Ctx, baseseed+i, s.Client, s.Miner, carExport, fastRet, startEpoch)
	}
}

func runFastRetrievalDealFlowT(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	s := kit.SetupOneClientOneMiner(t, b, blocktime)
	defer s.BlockMiner.Stop()

	data := make([]byte, 1600)
	rand.New(rand.NewSource(int64(8))).Read(data)

	r := bytes.NewReader(data)
	fcid, err := s.Client.ClientImportLocal(s.Ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	deal := kit.StartDeal(t, s.Ctx, s.Miner, s.Client, fcid, true, startEpoch)

	kit.WaitDealPublished(t, s.Ctx, s.Miner, deal)
	fmt.Println("deal published, retrieving")
	// Retrieval
	info, err := s.Client.ClientGetDealInfo(s.Ctx, *deal)
	require.NoError(t, err)

	kit.TestRetrieval(t, s.Ctx, s.Client, fcid, &info.PieceCID, false, data)
}

func runSecondDealRetrievalTest(t *testing.T, b kit.APIBuilder, blocktime time.Duration) {
	s := kit.SetupOneClientOneMiner(t, b, blocktime)
	defer s.BlockMiner.Stop()

	{
		data1 := make([]byte, 800)
		rand.New(rand.NewSource(int64(3))).Read(data1)
		r := bytes.NewReader(data1)

		fcid1, err := s.Client.ClientImportLocal(s.Ctx, r)
		if err != nil {
			t.Fatal(err)
		}

		data2 := make([]byte, 800)
		rand.New(rand.NewSource(int64(9))).Read(data2)
		r2 := bytes.NewReader(data2)

		fcid2, err := s.Client.ClientImportLocal(s.Ctx, r2)
		if err != nil {
			t.Fatal(err)
		}

		deal1 := kit.StartDeal(t, s.Ctx, s.Miner, s.Client, fcid1, true, 0)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)
		kit.WaitDealSealed(t, s.Ctx, s.Miner, s.Client, deal1, true)

		deal2 := kit.StartDeal(t, s.Ctx, s.Miner, s.Client, fcid2, true, 0)

		time.Sleep(time.Second)
		kit.WaitDealSealed(t, s.Ctx, s.Miner, s.Client, deal2, false)

		// Retrieval
		info, err := s.Client.ClientGetDealInfo(s.Ctx, *deal2)
		require.NoError(t, err)

		rf, _ := s.Miner.SectorsRefs(s.Ctx)
		fmt.Printf("refs: %+v\n", rf)

		kit.TestRetrieval(t, s.Ctx, s.Client, fcid2, &info.PieceCID, false, data2)
	}
}

func runZeroPricePerByteRetrievalDealFlow(t *testing.T, b kit.APIBuilder, blocktime time.Duration, startEpoch abi.ChainEpoch) {
	s := kit.SetupOneClientOneMiner(t, b, blocktime)
	defer s.BlockMiner.Stop()

	// Set price-per-byte to zero
	ask, err := s.Miner.MarketGetRetrievalAsk(s.Ctx)
	require.NoError(t, err)

	ask.PricePerByte = abi.NewTokenAmount(0)
	err = s.Miner.MarketSetRetrievalAsk(s.Ctx, ask)
	require.NoError(t, err)

	kit.MakeDeal(t, s.Ctx, 6, s.Client, s.Miner, false, false, startEpoch)
}
