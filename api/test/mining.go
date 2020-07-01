package test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/impl"
)

var log = logging.Logger("apitest")

func (ts *testSuite) testMining(t *testing.T) {
	ctx := context.Background()
	apis, sn := ts.makeNodes(t, 1, oneMiner)
	api := apis[0]

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(0), h1.Height())

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	<-newHeads

	err = sn[0].MineOne(ctx, func(bool) {})
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(1), h2.Height())
}

func (ts *testSuite) testMiningReal(t *testing.T) {
	build.InsecurePoStValidation = false
	defer func() {
		build.InsecurePoStValidation = true
	}()

	ctx := context.Background()
	apis, sn := ts.makeNodes(t, 1, oneMiner)
	api := apis[0]

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(0), h1.Height())

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	<-newHeads

	err = sn[0].MineOne(ctx, func(bool) {})
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(1), h2.Height())

	err = sn[0].MineOne(ctx, func(bool) {})
	require.NoError(t, err)

	<-newHeads

	h2, err = api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(2), h2.Height())
}

func TestDealMining(t *testing.T, b APIBuilder, blocktime time.Duration, carExport bool) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	// test making a deal with a fresh miner, and see if it starts to mine

	ctx := context.Background()
	n, sn := b(t, 1, []StorageMiner{
		{Full: 0, Preseal: PresealGenesis},
		{Full: 0, Preseal: 0}, // TODO: Add support for storage miners on non-first full node
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

	go func() {
		doneMinedTwo := false
		defer close(done)

		prevExpect := 0
		for atomic.LoadInt32(&mine) != 0 {
			wait := make(chan int, 2)
			mdone := func(mined bool) {
				go func() {
					n := 0
					if mined {
						n = 1
					}
					wait <- n
				}()
			}

			if err := sn[0].MineOne(ctx, mdone); err != nil {
				t.Error(err)
			}

			if err := sn[1].MineOne(ctx, mdone); err != nil {
				t.Error(err)
			}

			expect := <-wait
			expect += <-wait

			time.Sleep(blocktime)

			for {
				n := 0
				for i, node := range sn {
					mb, err := node.MiningBase(ctx)
					if err != nil {
						t.Error(err)
						return
					}

					if len(mb.Cids()) != expect {
						log.Warnf("node %d mining base not complete (%d, want %d)", i, len(mb.Cids()), expect)
						continue
					}
					n++
				}
				if n == len(sn) {
					break
				}
				time.Sleep(blocktime)
			}

			if prevExpect == 2 && expect == 2 && !doneMinedTwo {
				close(minedTwo)
				doneMinedTwo = true
			}

			prevExpect = expect
		}
	}()

	deal := startDeal(t, ctx, provider, client, fcid)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	waitDealSealed(t, ctx, client, deal)

	<-minedTwo

	atomic.StoreInt32(&mine, 0)
	fmt.Println("shutting down mining")
	<-done
}
