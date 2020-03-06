package test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
)

func init() {
	logging.SetAllLoggers(logging.LevelInfo)
	build.InsecurePoStValidation = true
}

func TestDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration) {
	os.Setenv("BELLMAN_NO_GPU", "1")

	ctx := context.Background()
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

	data := make([]byte, 1600)
	rand.New(rand.NewSource(5)).Read(data)

	r := bytes.NewReader(data)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	mine := true
	done := make(chan struct{})

	go func() {
		defer close(done)
		for mine {
			time.Sleep(blocktime)
			if err := sn[0].MineOne(ctx); err != nil {
				t.Error(err)
			}
		}
	}()
	addr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deal, err := client.ClientStartDeal(ctx, &api.StartDealParams{
		Data:           &storagemarket.DataRef{Root: fcid},
		Wallet:         addr,
		Miner:          maddr,
		EpochPrice:     types.NewInt(1000000),
		BlocksDuration: 100,
	})
	if err != nil {
		t.Fatalf("%+v", err)
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
		case storagemarket.StorageDealProposalRejected:
			t.Fatal("deal rejected")
		case storagemarket.StorageDealFailing:
			t.Fatal("deal failed")
		case storagemarket.StorageDealError:
			t.Fatal("deal errored")
		case storagemarket.StorageDealActive:
			fmt.Println("COMPLETE", di)
			break loop
		}
		fmt.Println("Deal state: ", storagemarket.DealStates[di.State])
		time.Sleep(time.Second / 2)
	}

	// Retrieval

	offers, err := client.ClientFindData(ctx, fcid)
	if err != nil {
		t.Fatal(err)
	}

	if len(offers) < 1 {
		t.Fatal("no offers")
	}

	rpath, err := ioutil.TempDir("", "lotus-retrieve-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(rpath)

	caddr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ref := api.FileRef{
		Path:  filepath.Join(rpath, "ret"),
		IsCAR: false,
	}
	err = client.ClientRetrieve(ctx, offers[0].Order(caddr), ref)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	rdata, err := ioutil.ReadFile(filepath.Join(rpath, "ret"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(rdata, data) {
		t.Fatal("wrong data retrieved")
	}

	mine = false
	fmt.Println("shutting down mining")
	<-done
}
