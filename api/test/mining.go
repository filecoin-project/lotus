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

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/impl"
)

//nolint:deadcode,varcheck
var log = logging.Logger("apitest")

func (ts *testSuite) testMining(t *testing.T) {
	ctx := context.Background()
	apis, sn := ts.makeNodes(t, OneFull, OneMiner)
	api := apis[0]

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	initHead := (<-newHeads)[0]
	baseHeight := initHead.Val.Height()

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(h1.Height()), int64(baseHeight))

	MineUntilBlock(ctx, t, apis[0], sn[0], nil)
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(h2.Height()), int64(h1.Height()))
}

func (ts *testSuite) testMiningReal(t *testing.T) {
	build.InsecurePoStValidation = false
	defer func() {
		build.InsecurePoStValidation = true
	}()

	ctx := context.Background()
	apis, sn := ts.makeNodes(t, OneFull, OneMiner)
	api := apis[0]

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	initHead := (<-newHeads)[0]
	if initHead.Val.Height() != 2 {
		<-newHeads
	}

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(2), h1.Height())

	MineUntilBlock(ctx, t, apis[0], sn[0], nil)
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(3), h2.Height())

	MineUntilBlock(ctx, t, apis[0], sn[0], nil)
	require.NoError(t, err)

	<-newHeads

	h2, err = api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, abi.ChainEpoch(4), h2.Height())
}

func TestDealMining(t *testing.T, b APIBuilder, blocktime time.Duration, carExport bool) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	// test making a deal with a fresh miner, and see if it starts to mine

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

	deal := startDeal(t, ctx, provider, client, fcid, false)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	waitDealSealed(t, ctx, provider, client, deal, false)

	<-minedTwo

	atomic.StoreInt32(&mine, 0)
	fmt.Println("shutting down mining")
	<-done
}
