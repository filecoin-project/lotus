package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/repo"

	"github.com/filecoin-project/specs-actors/actors/crypto"
)

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
	t.D().Counter(fmt.Sprintf("send-data-to,miner=%s", minerAddr.ActorAddr)).Inc(1)

	t.RecordMessage("selected %s as the miner", minerAddr.ActorAddr)

	time.Sleep(2 * time.Second)

	// generate 1600 bytes of random data
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

	time.Sleep(10 * time.Second) // wait for metrics to be emitted

	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retrieve it

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func prepareClient(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := getPubsubTracerMaddr(ctx, t)
	if err != nil {
		return nil, err
	}

	drandOpt, err := getDrandOpts(ctx, t)
	if err != nil {
		return nil, err
	}

	// first create a wallet
	walletKey, err := wallet.GenerateKey(crypto.SigTypeBLS)
	if err != nil {
		return nil, err
	}

	// publish the account ID/balance
	balance := t.IntParam("balance")
	balanceMsg := &InitialBalanceMsg{Addr: walletKey.Address, Balance: balance}
	t.SyncClient.Publish(ctx, balanceTopic, balanceMsg)

	// then collect the genesis block and bootstrapper address
	genesisMsg, err := waitForGenesis(t, ctx)
	if err != nil {
		return nil, err
	}

	clientIP := t.NetClient.MustGetDataNetworkIP().String()

	nodeRepo := repo.NewMemory(nil)

	// create the node
	n := &Node{}
	stop, err := node.New(context.Background(),
		node.FullAPI(&n.fullApi),
		node.Online(),
		node.Repo(nodeRepo),
		withApiEndpoint("/ip4/127.0.0.1/tcp/1234"),
		withGenesis(genesisMsg.Genesis),
		withListenAddress(clientIP),
		withBootstrapper(genesisMsg.Bootstrapper),
		withPubsubConfig(false, pubsubTracer),
		drandOpt,
	)
	if err != nil {
		return nil, err
	}
	n.stop = stop

	// set the wallet
	err = n.setWallet(ctx, walletKey)
	if err != nil {
		stop(context.TODO())
		return nil, err
	}

	err = startClientAPIServer(nodeRepo, n.fullApi)
	if err != nil {
		return nil, err
	}

	registerAndExportMetrics(fmt.Sprintf("client_%d", t.GroupSeq))

	t.RecordMessage("publish our address to the clients addr topic")
	addrinfo, err := n.fullApi.NetAddrsListen(ctx)
	if err != nil {
		return nil, err
	}
	t.SyncClient.MustPublish(ctx, clientsAddrsTopic, addrinfo)

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, stateReady, t.TestInstanceCount)

	return n, nil
}

func startClientAPIServer(repo *repo.MemRepo, api api.FullNode) error {
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", apistruct.PermissionedFullAPI(api))

	ah := &auth.Handler{
		Verify: api.AuthVerify,
		Next:   rpcServer.ServeHTTP,
	}

	http.Handle("/rpc/v0", ah)

	srv := &http.Server{Handler: http.DefaultServeMux}

	return startServer(repo, srv)
}
