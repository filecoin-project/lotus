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
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	dstest "github.com/ipfs/go-merkledag/test"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
	"github.com/libp2p/go-libp2p-core/peer"
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
	"bootstrapper": runBaselineBootstrapper,
	"miner":        runBaselineMiner,
	"client":       runBaselineClient,
}

func runBaselineBootstrapper(t *TestEnvironment) error {
	t.RecordMessage("running bootstrapper")
	_, err := prepareBootstrapper(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func runBaselineMiner(t *TestEnvironment) error {
	t.RecordMessage("running miner")
	miner, err := prepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	addrs, err := collectClientsAddrs(t, ctx, t.IntParam("clients"))
	if err != nil {
		return err
	}

	t.RecordMessage("got %v client addrs", len(addrs))

	// mine / stop mining
	mine := true
	done := make(chan struct{})
	go func() {
		defer close(done)
		for mine {
			time.Sleep(1000 * time.Millisecond)
			t.RecordMessage("mine one block")

			// wait and synchronise

			if err := miner.MineOne(ctx, func(bool) {

				// after a block is mined

			}); err != nil {
				panic(err)
			}
		}
	}()

	// wait for a signa to stop mining
	err = <-t.SyncClient.MustBarrier(ctx, stateStopMining, 1).C
	if err != nil {
		return err
	}

	mine = false
	t.RecordMessage("shutting down mining")
	<-done

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func runBaselineClient(t *TestEnvironment) error {
	t.RecordMessage("running client")
	cl, err := prepareClient(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	addrs, err := collectMinersAddrs(t, ctx, t.IntParam("miners"))
	if err != nil {
		return err
	}

	client := cl.fullApi.(*impl.FullNodeAPI)

	t.RecordMessage("got %v miner addrs", len(addrs))

	if err := client.NetConnect(ctx, addrs[0].PeerAddr); err != nil {
		return err
	}

	t.RecordMessage("client connected to miner")

	time.Sleep(2 * time.Second)

	// generate random data
	data := make([]byte, 1600)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(data)
	r := bytes.NewReader(data)
	fcid, err := client.ClientImportLocal(ctx, r)
	if err != nil {
		return err
	}
	t.RecordMessage("file cid: %s", fcid)

	// start deal
	deal := startDeal(ctx, addrs[0].ActorAddr, client, fcid)
	t.RecordMessage("started deal: %s", deal)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(2 * time.Second)

	t.RecordMessage("wait to be sealed")
	waitDealSealed(t, ctx, client, deal)

	carExport := true

	t.RecordMessage("try to retrieve fcid")
	retrieve(t, ctx, err, client, fcid, carExport, data)

	t.SyncClient.MustSignalEntry(ctx, stateStopMining)

	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retrieve it

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func collectMinersAddrs(t *TestEnvironment, ctx context.Context, miners int) ([]MinerAddresses, error) {
	ch := make(chan MinerAddresses)
	sub := t.SyncClient.MustSubscribe(ctx, minersAddrsTopic, ch)

	addrs := make([]MinerAddresses, 0, miners)
	for i := 0; i < miners; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for miners addrs: %w", err)
		}
	}

	return addrs, nil
}

func collectClientsAddrs(t *TestEnvironment, ctx context.Context, clients int) ([]peer.AddrInfo, error) {
	ch := make(chan peer.AddrInfo)
	sub := t.SyncClient.MustSubscribe(ctx, clientsAddrsTopic, ch)

	addrs := make([]peer.AddrInfo, 0, clients)
	for i := 0; i < clients; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for clients addrs: %w", err)
		}
	}

	return addrs, nil
}

func startDeal(ctx context.Context, minerActorAddr address.Address, client *impl.FullNodeAPI, fcid cid.Cid) *cid.Cid {
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

func waitDealSealed(t *TestEnvironment, ctx context.Context, client *impl.FullNodeAPI, deal *cid.Cid) {
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

func retrieve(t *TestEnvironment, ctx context.Context, err error, client *impl.FullNodeAPI, fcid cid.Cid, carExport bool, data []byte) {
	offers, err := client.ClientFindData(ctx, fcid)
	if err != nil {
		panic(err)
	}

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
	err = client.ClientRetrieve(ctx, offers[0].Order(caddr), ref)
	if err != nil {
		panic(err)
	}

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
