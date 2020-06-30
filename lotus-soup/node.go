package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/beacon"
	genesis_chain "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	influxdb "github.com/kpacha/opencensus-influxdb"

	libp2p_crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

func init() {
	logging.SetLogLevel("*", "ERROR")

	os.Setenv("BELLMAN_NO_GPU", "1")

	build.InsecurePoStValidation = true
	build.DisableBuiltinAssets = true

	power.ConsensusMinerMinPower = big.NewInt(2048)
	saminer.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}

var (
	PrepareNodeTimeout = time.Minute

	genesisTopic = sync.NewTopic("genesis", &GenesisMsg{})
	balanceTopic = sync.NewTopic("balance", &InitialBalanceMsg{})
	presealTopic = sync.NewTopic("preseal", &PresealMsg{})

	clientsAddrsTopic = sync.NewTopic("clientsAddrsTopic", &peer.AddrInfo{})
	minersAddrsTopic  = sync.NewTopic("minersAddrsTopic", &MinerAddresses{})

	stateReady           = sync.State("ready")
	stateDone            = sync.State("done")
	stateStopMining      = sync.State("stop-mining")
	stateMinerPickSeqNum = sync.State("miner-pick-seq-num")
)

type TestEnvironment struct {
	*runtime.RunEnv
	*run.InitContext
}

// workaround for default params being wrapped in quote chars
func (t *TestEnvironment) StringParam(name string) string {
	return strings.Trim(t.RunEnv.StringParam(name), "\"")
}

func (t *TestEnvironment) DurationParam(name string) time.Duration {
	d, err := time.ParseDuration(t.StringParam(name))
	if err != nil {
		panic(fmt.Errorf("invalid duration value for param '%s': %w", name, err))
	}
	return d
}

type Node struct {
	fullApi  api.FullNode
	minerApi api.StorageMiner
	stop     node.StopFunc
	MineOne  func(context.Context, func(bool)) error
}

type InitialBalanceMsg struct {
	Addr    address.Address
	Balance int
}

type PresealMsg struct {
	Miner genesis.Miner
	Seqno int64
}

type GenesisMsg struct {
	Genesis      []byte
	Bootstrapper []byte
}

type MinerAddresses struct {
	PeerAddr  peer.AddrInfo
	ActorAddr address.Address
}

func prepareBootstrapper(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := getPubsubTracerConfig(ctx, t)
	if err != nil {
		return nil, err
	}

	clients := t.IntParam("clients")
	miners := t.IntParam("miners")
	nodes := clients + miners

	drandOpt, err := getDrandConfig(ctx, t)
	if err != nil {
		return nil, err
	}

	// the first duty of the boostrapper is to construct the genesis block
	// first collect all client and miner balances to assign initial funds
	balances, err := waitForBalances(t, ctx, nodes)
	if err != nil {
		return nil, err
	}

	// then collect all preseals from miners
	preseals, err := collectPreseals(t, ctx, miners)
	if err != nil {
		return nil, err
	}

	// now construct the genesis block
	var genesisActors []genesis.Actor
	var genesisMiners []genesis.Miner

	for _, bm := range balances {
		genesisActors = append(genesisActors,
			genesis.Actor{
				Type:    genesis.TAccount,
				Balance: big.Mul(big.NewInt(int64(bm.Balance)), types.NewInt(build.FilecoinPrecision)),
				Meta:    (&genesis.AccountMeta{Owner: bm.Addr}).ActorMeta(),
			})
	}

	for _, pm := range preseals {
		genesisMiners = append(genesisMiners, pm.Miner)
	}

	genesisTemplate := genesis.Template{
		Accounts:  genesisActors,
		Miners:    genesisMiners,
		Timestamp: uint64(time.Now().Unix()) - uint64(t.IntParam("genesis_timestamp_offset")), // this needs to be in the past
	}

	// dump the genesis block
	// var jsonBuf bytes.Buffer
	// jsonEnc := json.NewEncoder(&jsonBuf)
	// err := jsonEnc.Encode(genesisTemplate)
	// if err != nil {
	// 	panic(err)
	// }
	// runenv.RecordMessage(fmt.Sprintf("Genesis template: %s", string(jsonBuf.Bytes())))

	// this is horrendously disgusting, we use this contraption to side effect the construction
	// of the genesis block in the buffer -- yes, a side effect of dependency injection.
	// I remember when software was straightforward...
	var genesisBuffer bytes.Buffer

	bootstrapperIP := t.NetClient.MustGetDataNetworkIP().String()

	n := &Node{}
	stop, err := node.New(context.Background(),
		node.FullAPI(&n.fullApi),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genesisBuffer, genesisTemplate)),
		withApiEndpoint("/ip4/127.0.0.1/tcp/1234"),
		withListenAddress(bootstrapperIP),
		withBootstrapper(nil),
		withPubsubConfig(true, pubsubTracer),
		drandOpt,
	)
	if err != nil {
		return nil, err
	}
	n.stop = stop

	var bootstrapperAddr ma.Multiaddr

	bootstrapperAddrs, err := n.fullApi.NetAddrsListen(ctx)
	if err != nil {
		stop(context.TODO())
		return nil, err
	}
	for _, a := range bootstrapperAddrs.Addrs {
		ip, err := a.ValueForProtocol(ma.P_IP4)
		if err != nil {
			continue
		}
		if ip != bootstrapperIP {
			continue
		}
		addrs, err := peer.AddrInfoToP2pAddrs(&peer.AddrInfo{
			ID:    bootstrapperAddrs.ID,
			Addrs: []ma.Multiaddr{a},
		})
		if err != nil {
			panic(err)
		}
		bootstrapperAddr = addrs[0]
		break
	}

	if bootstrapperAddr == nil {
		panic("failed to determine bootstrapper address")
	}

	genesisMsg := &GenesisMsg{
		Genesis:      genesisBuffer.Bytes(),
		Bootstrapper: bootstrapperAddr.Bytes(),
	}
	t.SyncClient.MustPublish(ctx, genesisTopic, genesisMsg)

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, stateReady, t.TestInstanceCount)

	return n, nil
}

func prepareMiner(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := getPubsubTracerConfig(ctx, t)
	if err != nil {
		return nil, err
	}

	drandOpt, err := getDrandConfig(ctx, t)
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

	// create and publish the preseal commitment
	priv, _, err := libp2p_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	minerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	// pick unique sequence number for each miner, no matter in which group they are
	seq := t.SyncClient.MustSignalAndWait(ctx, stateMinerPickSeqNum, t.IntParam("miners"))

	minerAddr, err := address.NewIDAddress(genesis_chain.MinerStart + uint64(seq-1))
	if err != nil {
		return nil, err
	}

	presealDir, err := ioutil.TempDir("", "preseal")
	if err != nil {
		return nil, err
	}

	sectors := t.IntParam("sectors")
	genMiner, _, err := seed.PreSeal(minerAddr, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, sectors, presealDir, []byte("TODO: randomize this"), &walletKey.KeyInfo)
	if err != nil {
		return nil, err
	}
	genMiner.PeerId = minerID

	t.RecordMessage("Miner Info: Owner: %s Worker: %s", genMiner.Owner, genMiner.Worker)

	presealMsg := &PresealMsg{Miner: *genMiner, Seqno: seq}
	t.SyncClient.Publish(ctx, presealTopic, presealMsg)

	// then collect the genesis block and bootstrapper address
	genesisMsg, err := waitForGenesis(t, ctx)
	if err != nil {
		return nil, err
	}

	// prepare the repo
	minerRepo := repo.NewMemory(nil)

	lr, err := minerRepo.Lock(repo.StorageMiner)
	if err != nil {
		return nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, err
	}

	kbytes, err := priv.Bytes()
	if err != nil {
		return nil, err
	}

	err = ks.Put("libp2p-host", types.KeyInfo{
		Type:       "libp2p-host",
		PrivateKey: kbytes,
	})
	if err != nil {
		return nil, err
	}

	ds, err := lr.Datastore("/metadata")
	if err != nil {
		return nil, err
	}

	err = ds.Put(datastore.NewKey("miner-address"), minerAddr.Bytes())
	if err != nil {
		return nil, err
	}

	nic := storedcounter.New(ds, datastore.NewKey(modules.StorageCounterDSPrefix))
	for i := 0; i < (sectors + 1); i++ {
		_, err = nic.Next()
		if err != nil {
			return nil, err
		}
	}

	err = lr.Close()
	if err != nil {
		return nil, err
	}

	minerIP := t.NetClient.MustGetDataNetworkIP().String()

	// create the node
	// we need both a full node _and_ and storage miner node
	n := &Node{}

	nodeRepo := repo.NewMemory(nil)

	stop1, err := node.New(context.Background(),
		node.FullAPI(&n.fullApi),
		node.Online(),
		node.Repo(nodeRepo),
		withGenesis(genesisMsg.Genesis),
		withListenAddress(minerIP),
		withBootstrapper(genesisMsg.Bootstrapper),
		withPubsubConfig(false, pubsubTracer),
		drandOpt,
	)
	if err != nil {
		return nil, err
	}

	// set the wallet
	err = n.setWallet(ctx, walletKey)
	if err != nil {
		stop1(context.TODO())
		return nil, err
	}

	minerOpts := []node.Option{
		node.StorageMiner(&n.minerApi),
		node.Online(),
		node.Repo(minerRepo),
		node.Override(new(api.FullNode), n.fullApi),
		withApiEndpoint("/ip4/127.0.0.1/tcp/1234"),
		withMinerListenAddress(minerIP),
	}

	if t.StringParam("mining_mode") != "natural" {
		mineBlock := make(chan func(bool))
		minerOpts = append(minerOpts,
			node.Override(new(*miner.Miner), miner.NewTestMiner(mineBlock, minerAddr)))
		n.MineOne = func(ctx context.Context, cb func(bool)) error {
			select {
			case mineBlock <- cb:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	stop2, err := node.New(context.Background(), minerOpts...)
	if err != nil {
		stop1(context.TODO())
		return nil, err
	}
	n.stop = func(ctx context.Context) error {
		// TODO use a multierror for this
		err2 := stop2(ctx)
		err1 := stop1(ctx)
		if err2 != nil {
			return err2
		}
		return err1
	}

	registerAndExportMetrics(minerAddr.String())

	// Bootstrap with full node
	remoteAddrs, err := n.fullApi.NetAddrsListen(ctx)
	if err != nil {
		panic(err)
	}

	err = n.minerApi.NetConnect(ctx, remoteAddrs)
	if err != nil {
		panic(err)
	}

	err = startStorMinerAPIServer(minerRepo, n.minerApi)
	if err != nil {
		return nil, err
	}

	// add local storage for presealed sectors
	err = n.minerApi.StorageAddLocal(ctx, presealDir)
	if err != nil {
		n.stop(context.TODO())
		return nil, err
	}

	// set the miner PeerID
	minerIDEncoded, err := actors.SerializeParams(&saminer.ChangePeerIDParams{NewID: abi.PeerID(minerID)})
	if err != nil {
		return nil, err
	}

	changeMinerID := &types.Message{
		To:       minerAddr,
		From:     genMiner.Worker,
		Method:   builtin.MethodsMiner.ChangePeerID,
		Params:   minerIDEncoded,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
	}

	_, err = n.fullApi.MpoolPushMessage(ctx, changeMinerID)
	if err != nil {
		n.stop(context.TODO())
		return nil, err
	}

	t.RecordMessage("publish our address to the miners addr topic")
	actoraddress, err := n.minerApi.ActorAddress(ctx)
	if err != nil {
		return nil, err
	}
	addrinfo, err := n.minerApi.NetAddrsListen(ctx)
	if err != nil {
		return nil, err
	}
	t.SyncClient.MustPublish(ctx, minersAddrsTopic, MinerAddresses{addrinfo, actoraddress})

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, stateReady, t.TestInstanceCount)

	return n, err
}

func prepareClient(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := getPubsubTracerConfig(ctx, t)
	if err != nil {
		return nil, err
	}

	drandOpt, err := getDrandConfig(ctx, t)
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

func (n *Node) setWallet(ctx context.Context, walletKey *wallet.Key) error {
	_, err := n.fullApi.WalletImport(ctx, &walletKey.KeyInfo)
	if err != nil {
		return err
	}

	err = n.fullApi.WalletSetDefault(ctx, walletKey.Address)
	if err != nil {
		return err
	}

	return nil
}

func withGenesis(gb []byte) node.Option {
	return node.Override(new(modules.Genesis), modules.LoadGenesis(gb))
}

func withBootstrapper(ab []byte) node.Option {
	return node.Override(new(dtypes.BootstrapPeers),
		func() (dtypes.BootstrapPeers, error) {
			if ab == nil {
				return dtypes.BootstrapPeers{}, nil
			}

			a, err := ma.NewMultiaddrBytes(ab)
			if err != nil {
				return nil, err
			}
			ai, err := peer.AddrInfoFromP2pAddr(a)
			if err != nil {
				return nil, err
			}
			return dtypes.BootstrapPeers{*ai}, nil
		})
}

func withPubsubConfig(bootstrapper bool, pubsubTracer string) node.Option {
	return node.Override(new(*config.Pubsub), func() *config.Pubsub {
		return &config.Pubsub{
			Bootstrapper: bootstrapper,
			RemoteTracer: pubsubTracer,
		}
	})
}

func withListenAddress(ip string) node.Option {
	addrs := []string{fmt.Sprintf("/ip4/%s/tcp/4001", ip)}
	return node.Override(node.StartListeningKey, lp2p.StartListening(addrs))
}

func withMinerListenAddress(ip string) node.Option {
	addrs := []string{fmt.Sprintf("/ip4/%s/tcp/4002", ip)}
	return node.Override(node.StartListeningKey, lp2p.StartListening(addrs))
}

func withApiEndpoint(addr string) node.Option {
	return node.Override(node.SetApiEndpointKey, func(lr repo.LockedRepo) error {
		apima, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return err
		}
		return lr.SetAPIEndpoint(apima)
	})
}

func waitForBalances(t *TestEnvironment, ctx context.Context, nodes int) ([]*InitialBalanceMsg, error) {
	ch := make(chan *InitialBalanceMsg)
	sub := t.SyncClient.MustSubscribe(ctx, balanceTopic, ch)

	balances := make([]*InitialBalanceMsg, 0, nodes)
	for i := 0; i < nodes; i++ {
		select {
		case m := <-ch:
			balances = append(balances, m)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for balances: %w", err)
		}
	}

	return balances, nil
}

func collectPreseals(t *TestEnvironment, ctx context.Context, miners int) ([]*PresealMsg, error) {
	ch := make(chan *PresealMsg)
	sub := t.SyncClient.MustSubscribe(ctx, presealTopic, ch)

	preseals := make([]*PresealMsg, 0, miners)
	for i := 0; i < miners; i++ {
		select {
		case m := <-ch:
			preseals = append(preseals, m)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for preseals: %w", err)
		}
	}

	sort.Slice(preseals, func(i, j int) bool {
		return preseals[i].Seqno < preseals[j].Seqno
	})

	return preseals, nil
}

func waitForGenesis(t *TestEnvironment, ctx context.Context) (*GenesisMsg, error) {
	genesisCh := make(chan *GenesisMsg)
	sub := t.SyncClient.MustSubscribe(ctx, genesisTopic, genesisCh)

	select {
	case genesisMsg := <-genesisCh:
		return genesisMsg, nil
	case err := <-sub.Done():
		return nil, fmt.Errorf("error while waiting for genesis msg: %w", err)
	}
}

func collectMinerAddrs(t *TestEnvironment, ctx context.Context, miners int) ([]MinerAddresses, error) {
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

func collectClientAddrs(t *TestEnvironment, ctx context.Context, clients int) ([]peer.AddrInfo, error) {
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

func getPubsubTracerConfig(ctx context.Context, t *TestEnvironment) (string, error) {
	if !t.BooleanParam("enable_pubsub_tracer") {
		return "", nil
	}

	ch := make(chan *PubsubTracerMsg)
	sub := t.SyncClient.MustSubscribe(ctx, pubsubTracerTopic, ch)

	select {
	case m := <-ch:
		return m.Tracer, nil
	case err := <-sub.Done():
		return "", fmt.Errorf("got error while waiting for pubsub tracer config: %w", err)
	}
}

func getDrandConfig(ctx context.Context, t *TestEnvironment) (node.Option, error) {
	beaconType := t.StringParam("random_beacon_type")
	switch beaconType {
	case "external-drand":
		noop := func(settings *node.Settings) error {
			return nil
		}
		return noop, nil

	case "local-drand":
		cfg, err := waitForDrandConfig(ctx, t.SyncClient)
		if err != nil {
			t.RecordMessage("error getting drand config: %w", err)
			return nil, err

		}
		t.RecordMessage("setting drand config: %v", cfg)
		return node.Options(
			node.Override(new(dtypes.DrandConfig), cfg.Config),
			node.Override(new(dtypes.DrandBootstrap), cfg.GossipBootstrap),
		), nil

	case "mock":
		return node.Options(
			node.Override(new(beacon.RandomBeacon), modtest.RandomBeacon),
			node.Override(new(dtypes.DrandConfig), dtypes.DrandConfig{
				ChainInfoJSON: "{\"Hash\":\"wtf\"}",
			}),
			node.Override(new(dtypes.DrandBootstrap), dtypes.DrandBootstrap{}),
		), nil

	default:
		return nil, fmt.Errorf("unknown random_beacon_type: %s", beaconType)
	}
}

func startStorMinerAPIServer(repo *repo.MemRepo, minerApi api.StorageMiner) error {
	mux := mux.NewRouter()

	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", apistruct.PermissionedStorMinerAPI(minerApi))

	mux.Handle("/rpc/v0", rpcServer)
	mux.PathPrefix("/remote").HandlerFunc(minerApi.(*impl.StorageMinerAPI).ServeRemote)
	mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

	ah := &auth.Handler{
		Verify: minerApi.AuthVerify,
		Next:   mux.ServeHTTP,
	}

	srv := &http.Server{Handler: ah}

	return startServer(repo, srv)
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

func startServer(repo *repo.MemRepo, srv *http.Server) error {
	endpoint, err := repo.APIEndpoint()
	if err != nil {
		return err
	}

	lst, err := manet.Listen(endpoint)
	if err != nil {
		return fmt.Errorf("could not listen: %w", err)
	}

	go func() {
		_ = srv.Serve(manet.NetListener(lst))
	}()

	return nil
}

func registerAndExportMetrics(instanceName string) {
	// Register all Lotus metric views
	err := view.Register(metrics.DefaultViews...)
	if err != nil {
		panic(err)
	}

	// Set the metric to one so it is published to the exporter
	stats.Record(context.Background(), metrics.LotusInfo.M(1))

	// Register our custom exporter to opencensus
	e, err := influxdb.NewExporter(context.Background(), influxdb.Options{
		Database:     "testground",
		Address:      os.Getenv("INFLUXDB_URL"),
		Username:     "",
		Password:     "",
		InstanceName: instanceName,
	})
	if err != nil {
		panic(err)
	}
	view.RegisterExporter(e)
	view.SetReportingPeriod(5 * time.Second)
}
