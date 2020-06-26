package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
)

// This is the basline test; Filecoin 101.
//
// A network with a bootstrapper, a number of miners, and a number of clients/full nodes
// is constructed and connected through the bootstrapper.
// Some funds are allocated to each node and a number of sectors are presealed in the genesis block.
//
// The test plan:
// One or more clients store content to one or more miners, testing storage deals.
// The plan ensures that the storage deals hit the blockchain and measure the time it took.
// Verification: one or more clients retrieve and verify the hashes of stored content.
// The plan ensures that all (previously) published content can be correctly retrieved
// and measures the time it took.
//
// Preparation of the genesis block: this is the responsibility of the bootstrapper.
// In order to compute the genesis block, we need to collect identities and presealed
// sectors from each node.
// The we create a genesis block that allocates some funds to each node and collects
// the presealed sectors.
var baselineRoles = map[string]func(*TestEnvironment) error{
	"bootstrapper":  runBootstrapper,
	"miner":         runMiner,
	"client":        runBaselineClient,
	"drand":         runDrandNode,
	"pubsub-tracer": runPubsubTracer,
}

func runBaselineClient(t *TestEnvironment) error {
	t.RecordMessage("running client")
	cl, err := prepareClient(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	addrs, err := collectMinerAddrs(t, ctx, t.IntParam("miners"))
	if err != nil {
		return err
	}
	t.RecordMessage("got %v miner addrs", len(addrs))

	client := cl.fullApi

	// select a random miner
	minerAddr := addrs[rand.Intn(len(addrs))]
	if err := client.NetConnect(ctx, minerAddr.PeerAddr); err != nil {
		return err
	}

	t.RecordMessage("selected %s as the miner", minerAddr.ActorAddr)

	time.Sleep(2 * time.Second)

	// generate random data
	data := make([]byte, 1600)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(data)

	file, err := ioutil.TempFile("/tmp", "data")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	fcid, err := client.ClientImport(ctx, api.FileRef{Path: file.Name(), IsCAR: false})
	if err != nil {
		return err
	}
	t.RecordMessage("file cid: %s", fcid)

	// start deal
	t1 := time.Now()
	deal := startDeal(ctx, minerAddr.ActorAddr, client, fcid)
	t.RecordMessage("started deal: %s", deal)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(2 * time.Second)

	t.RecordMessage("waiting for deal to be sealed")
	waitDealSealed(t, ctx, client, deal)
	t.D().ResettingHistogram("deal.sealed").Update(int64(time.Since(t1)))

	carExport := true

	t.RecordMessage("trying to retrieve %s", fcid)
	retrieveData(t, ctx, err, client, fcid, carExport, data)
	t.D().ResettingHistogram("deal.retrieved").Update(int64(time.Since(t1)))

	t.SyncClient.MustSignalEntry(ctx, stateStopMining)

	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retrieve it

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func startDeal(ctx context.Context, minerActorAddr address.Address, client api.FullNode, fcid cid.Cid) *cid.Cid {
	addr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		panic(err)
	}

	deal, err := client.ClientStartDeal(ctx, &api.StartDealParams{
		Data:              &storagemarket.DataRef{Root: fcid},
		Wallet:            addr,
		Miner:             minerActorAddr,
		EpochPrice:        types.NewInt(1000000),
		MinBlocksDuration: 1000,
	})
	if err != nil {
		panic(err)
	}
	return deal
}

func waitDealSealed(t *TestEnvironment, ctx context.Context, client api.FullNode, deal *cid.Cid) {
loop:
	for {
		di, err := client.ClientGetDealInfo(ctx, *deal)
		if err != nil {
			panic(err)
		}
		switch di.State {
		case storagemarket.StorageDealProposalRejected:
			panic("deal rejected")
		case storagemarket.StorageDealFailing:
			panic("deal failed")
		case storagemarket.StorageDealError:
			panic(fmt.Sprintf("deal errored %s", di.Message))
		case storagemarket.StorageDealActive:
			t.RecordMessage("completed deal: %s", di)
			break loop
		}
		t.RecordMessage("deal state: %s", storagemarket.DealStates[di.State])
		time.Sleep(2 * time.Second)
	}
}

func retrieveData(t *TestEnvironment, ctx context.Context, err error, client api.FullNode, fcid cid.Cid, carExport bool, data []byte) {
	t1 := time.Now()
	offers, err := client.ClientFindData(ctx, fcid)
	if err != nil {
		panic(err)
	}
	for _, o := range offers {
		t.D().Counter(fmt.Sprintf("find-data.offer.%s", o.Miner)).Inc(1)
	}
	t.D().ResettingHistogram("find-data").Update(int64(time.Since(t1)))

	if len(offers) < 1 {
		panic("no offers")
	}

	rpath, err := ioutil.TempDir("", "lotus-retrieve-test-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(rpath)

	caddr, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		panic(err)
	}

	ref := &api.FileRef{
		Path:  filepath.Join(rpath, "ret"),
		IsCAR: carExport,
	}
	t1 = time.Now()
	err = client.ClientRetrieve(ctx, offers[0].Order(caddr), ref)
	if err != nil {
		panic(err)
	}
	t.D().ResettingHistogram("retrieve-data").Update(int64(time.Since(t1)))

	rdata, err := ioutil.ReadFile(filepath.Join(rpath, "ret"))
	if err != nil {
		panic(err)
	}

	if carExport {
		rdata = extractCarData(ctx, rdata, rpath)
	}

	if !bytes.Equal(rdata, data) {
		panic("wrong data retrieved")
	}

	t.RecordMessage("retrieved successfully")
}

func extractCarData(ctx context.Context, rdata []byte, rpath string) []byte {
	bserv := dstest.Bserv()
	ch, err := car.LoadCar(bserv.Blockstore(), bytes.NewReader(rdata))
	if err != nil {
		panic(err)
	}
	b, err := bserv.GetBlock(ctx, ch.Roots[0])
	if err != nil {
		panic(err)
	}
	nd, err := ipld.Decode(b)
	if err != nil {
		panic(err)
	}
	dserv := dag.NewDAGService(bserv)
	fil, err := unixfile.NewUnixfsFile(ctx, dserv, nd)
	if err != nil {
		panic(err)
	}
	outPath := filepath.Join(rpath, "retLoadedCAR")
	if err := files.WriteTo(fil, outPath); err != nil {
		panic(err)
	}
	rdata, err = ioutil.ReadFile(outPath)
	if err != nil {
		panic(err)
	}
	return rdata
}
