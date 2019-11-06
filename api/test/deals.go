package test

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestDealFlow(t *testing.T, b APIBuilder) {
	ctx := context.TODO()
	n, sn := b(t, 1, []int{0})
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]
	_ = miner

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	addrinfo.Addrs = nil

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
		time.Sleep(time.Second)
		fmt.Println("mining a block now")
		if err := n[0].MineOne(ctx); err != nil {
			t.Fatal(err)
		}
		fmt.Println("mined a block")

		time.Sleep(time.Second)
		fmt.Println("mining a block now")
		if err := n[0].MineOne(ctx); err != nil {
			t.Fatal(err)
		}

	}()
	deal, err := client.ClientStartDeal(ctx, fcid, maddr, types.NewInt(1), 100)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Deal done!", deal)

	time.Sleep(time.Second * 10)
}
