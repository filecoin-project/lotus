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

	"github.com/ipfs/go-cid"

	files "github.com/ipfs/go-ipfs-files"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
	ipld "github.com/ipfs/go-ipld-format"
)

func init() {
	logging.SetAllLoggers(logging.LevelInfo)
	build.InsecurePoStValidation = true
}

func TestDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration, carExport bool) {
	os.Setenv("BELLMAN_NO_GPU", "1")

	ctx := context.Background()
	n, sn := b(t, 1, oneMiner)
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

	mine := true
	done := make(chan struct{})
	go func() {
		defer close(done)
		for mine {
			time.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, func(bool) {}); err != nil {
				t.Error(err)
			}
		}
	}()

	makeDeal(t, ctx, 6, client, miner, carExport)

	mine = false
	fmt.Println("shutting down mining")
	<-done
}

func TestDoubleDealFlow(t *testing.T, b APIBuilder, blocktime time.Duration) {
	os.Setenv("BELLMAN_NO_GPU", "1")

	ctx := context.Background()
	n, sn := b(t, 1, oneMiner)
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

	mine := true
	done := make(chan struct{})

	go func() {
		defer close(done)
		for mine {
			time.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, func(bool) {}); err != nil {
				t.Error(err)
			}
		}
	}()

	makeDeal(t, ctx, 6, client, miner, false)
	makeDeal(t, ctx, 7, client, miner, false)

	mine = false
	fmt.Println("shutting down mining")
	<-done
}

func makeDeal(t *testing.T, ctx context.Context, rseed int, client *impl.FullNodeAPI, miner TestStorageNode, carExport bool) {
	data := make([]byte, 1600)
	rand.New(rand.NewSource(int64(rseed))).Read(data)

	r := bytes.NewReader(data)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("FILE CID: ", fcid)

	deal := startDeal(t, ctx, miner, client, fcid)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)
	waitDealSealed(t, ctx, client, deal)

	// Retrieval

	testRetrieval(t, ctx, err, client, fcid, carExport, data)
}

func startDeal(t *testing.T, ctx context.Context, miner TestStorageNode, client *impl.FullNodeAPI, fcid cid.Cid) *cid.Cid {
	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deal, err := client.ClientStartDeal(ctx, &api.StartDealParams{
		Data:              &storagemarket.DataRef{Root: fcid},
		Wallet:            addr,
		Miner:             maddr,
		EpochPrice:        types.NewInt(1000000),
		MinBlocksDuration: 100,
	})
	if err != nil {
		t.Fatalf("%+v", err)
	}
	return deal
}

func waitDealSealed(t *testing.T, ctx context.Context, client *impl.FullNodeAPI, deal *cid.Cid) {
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
			t.Fatal("deal errored", di.Message)
		case storagemarket.StorageDealActive:
			fmt.Println("COMPLETE", di)
			break loop
		}
		fmt.Println("Deal state: ", storagemarket.DealStates[di.State])
		time.Sleep(time.Second / 2)
	}
}

func testRetrieval(t *testing.T, ctx context.Context, err error, client *impl.FullNodeAPI, fcid cid.Cid, carExport bool, data []byte) {
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

	ref := &api.FileRef{
		Path:  filepath.Join(rpath, "ret"),
		IsCAR: carExport,
	}
	err = client.ClientRetrieve(ctx, offers[0].Order(caddr), ref)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	rdata, err := ioutil.ReadFile(filepath.Join(rpath, "ret"))
	if err != nil {
		t.Fatal(err)
	}

	if carExport {
		rdata = extractCarData(t, ctx, rdata, rpath)
	}

	if !bytes.Equal(rdata, data) {
		t.Fatal("wrong data retrieved")
	}
}

func extractCarData(t *testing.T, ctx context.Context, rdata []byte, rpath string) []byte {
	bserv := dstest.Bserv()
	ch, err := car.LoadCar(bserv.Blockstore(), bytes.NewReader(rdata))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bserv.GetBlock(ctx, ch.Roots[0])
	if err != nil {
		t.Fatal(err)
	}
	nd, err := ipld.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	dserv := dag.NewDAGService(bserv)
	fil, err := unixfile.NewUnixfsFile(ctx, dserv, nd)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(rpath, "retLoadedCAR")
	if err := files.WriteTo(fil, outPath); err != nil {
		t.Fatal(err)
	}
	rdata, err = ioutil.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	return rdata
}
