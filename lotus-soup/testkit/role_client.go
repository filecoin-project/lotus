package testkit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/repo"

	"github.com/filecoin-project/specs-actors/actors/crypto"
)

type LotusClient struct {
	*LotusNode

	t          *TestEnvironment
	MinerAddrs []MinerAddressesMsg
}

func PrepareClient(t *TestEnvironment) (*LotusClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := GetPubsubTracerMaddr(ctx, t)
	if err != nil {
		return nil, err
	}

	drandOpt, err := GetRandomBeaconOpts(ctx, t)
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
	t.SyncClient.Publish(ctx, BalanceTopic, balanceMsg)

	// then collect the genesis block and bootstrapper address
	genesisMsg, err := WaitForGenesis(t, ctx)
	if err != nil {
		return nil, err
	}

	clientIP := t.NetClient.MustGetDataNetworkIP().String()

	nodeRepo := repo.NewMemory(nil)

	// create the node
	n := &LotusNode{}
	stop, err := node.New(context.Background(),
		node.FullAPI(&n.FullApi),
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
	n.StopFn = stop

	// set the wallet
	err = n.setWallet(ctx, walletKey)
	if err != nil {
		_ = stop(context.TODO())
		return nil, err
	}

	err = startClientAPIServer(nodeRepo, n.FullApi)
	if err != nil {
		return nil, err
	}

	registerAndExportMetrics(fmt.Sprintf("client_%d", t.GroupSeq))

	t.RecordMessage("publish our address to the clients addr topic")
	addrinfo, err := n.FullApi.NetAddrsListen(ctx)
	if err != nil {
		return nil, err
	}
	t.SyncClient.MustPublish(ctx, ClientsAddrsTopic, addrinfo)

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, StateReady, t.TestInstanceCount)

	// collect miner addresses.
	addrs, err := CollectMinerAddrs(t, ctx, t.IntParam("miners"))
	if err != nil {
		return nil, err
	}
	t.RecordMessage("got %v miner addrs", len(addrs))

	cl := &LotusClient{
		t:          t,
		LotusNode:  n,
		MinerAddrs: addrs,
	}
	return cl, nil
}

func (c *LotusClient) RunDefault() error {
	// run forever
	c.t.RecordMessage("running default client forever")
	c.t.WaitUntilAllDone()
	return nil
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
