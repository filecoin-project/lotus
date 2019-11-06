package test

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestDealFlow(t *testing.T, b APIBuilder) {
	logging.SetAllLoggers(logging.LevelInfo)
	ctx := context.TODO()
	n, sn := b(t, 1, []int{0})
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)

	r := io.LimitReader(rand.New(rand.NewSource(17)), 350)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	maddr, err := address.NewFromString("t0101")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	go func() {
		for i := 0; i < 4; i++ {
			time.Sleep(time.Second)
			fmt.Println("mining a block now", i)
			if err := n[0].MineOne(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}()
	deal, err := client.ClientStartDeal(ctx, fcid, maddr, types.NewInt(200), 100)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
	for {
		di, err := client.ClientGetDealInfo(ctx, *deal)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("DEAL STATE: ", *deal, di.State)
		time.Sleep(time.Second / 2)
	}
	fmt.Println("Deal done!", deal)

	time.Sleep(time.Second * 10)
}
