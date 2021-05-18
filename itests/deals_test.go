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
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/stretchr/testify/require"
)

func TestDealCycle(t *testing.T) {
	QuietMiningLogs()

	blockTime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	dealStartEpoch := abi.ChainEpoch(2 << 12)

	t.Run("TestFullDealCycle_Single", func(t *testing.T) {
		runFullDealCycles(t, 1, MockSbBuilder, blockTime, false, false, dealStartEpoch)
	})
	t.Run("TestFullDealCycle_Two", func(t *testing.T) {
		runFullDealCycles(t, 2, MockSbBuilder, blockTime, false, false, dealStartEpoch)
	})
	t.Run("WithExportedCAR", func(t *testing.T) {
		runFullDealCycles(t, 1, MockSbBuilder, blockTime, true, false, dealStartEpoch)
	})
	t.Run("TestFastRetrievalDealCycle", func(t *testing.T) {
		TestFastRetrievalDealFlow(t, MockSbBuilder, blockTime, dealStartEpoch)
	})
}

func TestAPIDealFlowReal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	QuietMiningLogs()

	// TODO: just set this globally?
	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	t.Run("basic", func(t *testing.T) {
		runFullDealCycles(t, 1, Builder, time.Second, false, false, 0)
	})

	t.Run("fast-retrieval", func(t *testing.T) {
		runFullDealCycles(t, 1, Builder, time.Second, false, true, 0)
	})

	t.Run("retrieval-second", func(t *testing.T) {
		runSecondDealRetrievalTest(t, Builder, time.Second)
	})
}

func TestPublishDealsBatching(t *testing.T) {
	QuietMiningLogs()

	b := MockSbBuilder
	blocktime := 10 * time.Millisecond
	startEpoch := abi.ChainEpoch(2 << 12)

	publishPeriod := 10 * time.Second
	maxDealsPerMsg := uint64(2)

	// Set max deals per publish deals message to 2
	minerDef := []StorageMiner{{
		Full: 0,
		Opts: node.Override(
			new(*storageadapter.DealPublisher),
			storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
				Period:         publishPeriod,
				MaxDealsPerMsg: maxDealsPerMsg,
			})),
		Preseal: PresealGenesis,
	}}

	// Create a connect client and miner node
	n, sn := b(t, OneFull, minerDef)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]
	s := connectAndStartMining(t, blocktime, client, miner)
	defer s.blockMiner.Stop()

	// Starts a deal and waits until it's published
	runDealTillPublish := func(rseed int) {
		res, _, err := CreateClientFile(s.ctx, s.client, rseed)
		require.NoError(t, err)

		upds, err := client.ClientGetDealUpdates(s.ctx)
		require.NoError(t, err)

		startDeal(t, s.ctx, s.miner, s.client, res.Root, false, startEpoch)

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
	msgCids, err := s.client.StateListMessages(s.ctx, &api.MessageMatch{To: market.Address}, types.EmptyTSK, 1)
	require.NoError(t, err)
	count := 0
	for _, msgCid := range msgCids {
		msg, err := s.client.ChainGetMessage(s.ctx, msgCid)
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

	QuietMiningLogs()

	b := MockSbBuilder
	blocktime := 50 * time.Millisecond

	ctx := context.Background()
	n, sn := b(t, OneFull, []StorageMiner{
		{Full: 0, Preseal: PresealGenesis},
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

	deal := startDeal(t, ctx, provider, client, fcid, false, 0)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	waitDealSealed(t, ctx, provider, client, deal, false)

	<-minedTwo

	atomic.StoreInt32(&mine, 0)
	fmt.Println("shutting down mining")
	<-done
}

func runFullDealCycles(t *testing.T, n int, b APIBuilder, blocktime time.Duration, carExport, fastRet bool, startEpoch abi.ChainEpoch) {
	s := setupOneClientOneMiner(t, b, blocktime)
	defer s.blockMiner.Stop()

	baseseed := 6
	for i := 0; i < n; i++ {
		MakeDeal(t, s.ctx, baseseed+i, s.client, s.miner, carExport, fastRet, startEpoch)
	}
}
