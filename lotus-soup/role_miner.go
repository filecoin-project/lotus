package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	genesis_chain "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/gorilla/mux"
	libp2p_crypto "github.com/libp2p/go-libp2p-core/crypto"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/testground/sdk-go/sync"
)

func runMiner(t *TestEnvironment) error {
	t.RecordMessage("running miner")
	miner, err := prepareMiner(t)
	if err != nil {
		return err
	}

	t.RecordMessage("block delay: %v", build.BlockDelay)
	t.D().Gauge("miner.block-delay").Update(build.BlockDelay)

	ctx := context.Background()

	clients := t.IntParam("clients")
	miners := t.IntParam("miners")

	myActorAddr, err := miner.minerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	// mine / stop mining
	mine := true
	done := make(chan struct{})

	if miner.MineOne != nil {
		go func() {
			defer t.RecordMessage("shutting down mining")
			defer close(done)

			var i int
			for i = 0; mine; i++ {
				// synchronize all miners to mine the next block
				t.RecordMessage("synchronizing all miners to mine next block [%d]", i)
				stateMineNext := sync.State(fmt.Sprintf("mine-block-%d", i))
				t.SyncClient.MustSignalAndWait(ctx, stateMineNext, miners)

				ch := make(chan struct{})
				err := miner.MineOne(ctx, func(mined bool) {
					if mined {
						t.D().Counter(fmt.Sprintf("block.mine,miner=%s", myActorAddr)).Inc(1)
					}
					close(ch)
				})
				if err != nil {
					panic(err)
				}
				<-ch
			}

			// signal the last block to make sure no miners are left stuck waiting for the next block signal
			// while the others have stopped
			stateMineLast := sync.State(fmt.Sprintf("mine-block-%d", i))
			t.SyncClient.MustSignalEntry(ctx, stateMineLast)
		}()
	} else {
		close(done)
	}

	// wait for a signal from all clients to stop mining
	err = <-t.SyncClient.MustBarrier(ctx, stateStopMining, clients).C
	if err != nil {
		return err
	}

	mine = false
	<-done

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func prepareMiner(t *TestEnvironment) (*Node, error) {
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
	t.SyncClient.MustPublish(ctx, minersAddrsTopic, MinerAddressesMsg{addrinfo, actoraddress})

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, stateReady, t.TestInstanceCount)

	return n, err
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
