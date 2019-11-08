package test

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestDealFlow(t *testing.T, b APIBuilder) {
	os.Setenv("BELLMAN_NO_GPU", "1")

	logging.SetAllLoggers(logging.LevelInfo)
	ctx := context.Background()
	n, sn, cleanup := b(t, 1, []int{0})
	defer cleanup(ctx)
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

	r := io.LimitReader(rand.New(rand.NewSource(17)), 1000)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	maddr, err := address.NewFromString("t0101")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	mine := true
	done := make(chan struct{})

	go func() {
		defer close(done)
		for mine {
			time.Sleep(time.Second)
			fmt.Println("mining a block now")
			if err := n[0].MineOne(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}()
	deal, err := client.ClientStartDeal(ctx, fcid, maddr, types.NewInt(40000000), 100)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
loop:
	for {
		di, err := client.ClientGetDealInfo(ctx, *deal)
		if err != nil {
			t.Fatal(err)
		}
		switch di.State {
		case api.DealRejected:
			t.Fatal("deal rejected")
		case api.DealFailed:
			t.Fatal("deal failed")
		case api.DealError:
			t.Fatal("deal errored")
		case api.DealComplete:
			fmt.Println("COMPLETE", di)
			break loop
		}
		fmt.Println("Deal state: ", api.DealStates[di.State])
		time.Sleep(time.Second / 2)
	}

	mine = false
	fmt.Println("shutting down mining")
	<-done
}
