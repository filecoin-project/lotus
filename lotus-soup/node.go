package main

import (
	"bytes"
	"context"
	"crypto/rand"

	//"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"

	logging "github.com/ipfs/go-log/v2"
	libp2p_crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-storedcounter"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	genesis_chain "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

var (
	PrepareNodeTimeout = time.Minute

	genesisTopic = sync.NewTopic("genesis", &GenesisMsg{})
	balanceTopic = sync.NewTopic("balance", &InitialBalanceMsg{})
	presealTopic = sync.NewTopic("preseal", &PresealMsg{})

	stateReady = sync.State("ready")
)

type TestEnvironment struct {
	*runtime.RunEnv
	*run.InitContext
}

type Node struct {
	fullApi  api.FullNode
	minerApi api.StorageMiner
	stop     node.StopFunc
}

type InitialBalanceMsg struct {
	Addr    address.Address
	Balance int
}

type PresealMsg struct {
	Miner genesis.Miner
}

type GenesisMsg struct {
	Genesis      []byte
	Bootstrapper []byte
}

func init() {
	logging.SetLogLevel("vm", "WARN")

	build.DisableBuiltinAssets = true

	// Note: I don't understand the significance of this, but the node test does it.
	power.ConsensusMinerMinPower = big.NewInt(2048)
	saminer.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}

func prepareBootstrapper(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	clients := t.IntParam("clients")
	miners := t.IntParam("miners")
	nodes := clients + miners

	// the first duty of the boostrapper is to construct the genesis block
	// first collect all client and miner balances to assign initial funds
	balanceMsgs := make([]*InitialBalanceMsg, 0, nodes)
	balanceCh := make(chan *InitialBalanceMsg)

	t.SyncClient.MustSubscribe(ctx, balanceTopic, balanceCh)
	for i := 0; i < nodes; i++ {
		m := <-balanceCh
		balanceMsgs = append(balanceMsgs, m)
	}

	// then collect all preseals from miners
	presealMsgs := make([]*PresealMsg, 0, miners)
	presealCh := make(chan *PresealMsg)

	t.SyncClient.MustSubscribe(ctx, presealTopic, presealCh)
	for i := 0; i < miners; i++ {
		m := <-presealCh
		presealMsgs = append(presealMsgs, m)
	}

	// now construct the genesis block
	var genesisActors []genesis.Actor
	var genesisMiners []genesis.Miner

	for _, bm := range balanceMsgs {
		genesisActors = append(genesisActors,
			genesis.Actor{
				Type:    genesis.TAccount,
				Balance: big.Mul(big.NewInt(int64(bm.Balance)), types.NewInt(build.FilecoinPrecision)),
				Meta:    (&genesis.AccountMeta{Owner: bm.Addr}).ActorMeta(),
			})
	}

	for _, pm := range presealMsgs {
		genesisMiners = append(genesisMiners, pm.Miner)
	}

	genesisTemplate := genesis.Template{
		Accounts:  genesisActors,
		Miners:    genesisMiners,
		Timestamp: uint64(time.Now().Unix() - 1000), // this needs to be in the past
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
		withListenAddress(bootstrapperIP),
		withBootstrapper(nil),
		withPubsubConfig(true),
	)
	if err != nil {
		return nil, err
	}
	n.stop = stop

	// this dance to construct the bootstrapper multiaddr is quite vexing.
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

	// we are ready; wait for all nodes to be ready
	t.SyncClient.MustBarrier(ctx, stateReady, t.TestInstanceCount)

	return n, nil
}

func prepareMiner(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

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

	minerAddr, err := address.NewIDAddress(genesis_chain.MinerStart + uint64(t.GroupSeq-1))
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

	presealMsg := &PresealMsg{Miner: *genMiner}
	t.SyncClient.Publish(ctx, presealTopic, presealMsg)

	// then collect the genesis block and bootstrapper address
	genesisCh := make(chan *GenesisMsg)
	t.SyncClient.MustSubscribe(ctx, genesisTopic, genesisCh)
	genesisMsg := <-genesisCh

	// prepare the repo
	minerRepo := repo.NewMemory(nil)

	// V00D00 People DaNC3!
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

	stop1, err := node.New(context.Background(),
		node.FullAPI(&n.fullApi),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		withGenesis(genesisMsg.Genesis),
		withListenAddress(minerIP),
		withBootstrapper(genesisMsg.Bootstrapper),
		withPubsubConfig(false),
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

	stop2, err := node.New(context.Background(),
		node.StorageMiner(&n.minerApi),
		node.Online(),
		node.Repo(minerRepo),
		node.Override(new(api.FullNode), n.fullApi),
		withMinerListenAddress(minerIP),
	)
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

	// we are ready; wait for all nodes to be ready
	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustBarrier(ctx, stateReady, t.TestInstanceCount)

	return n, err
}

func prepareClient(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

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
	genesisCh := make(chan *GenesisMsg)
	t.SyncClient.MustSubscribe(ctx, genesisTopic, genesisCh)
	genesisMsg := <-genesisCh

	clientIP := t.NetClient.MustGetDataNetworkIP().String()

	// create the node
	n := &Node{}
	stop, err := node.New(context.Background(),
		node.FullAPI(&n.fullApi),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		withGenesis(genesisMsg.Genesis),
		withListenAddress(clientIP),
		withBootstrapper(genesisMsg.Bootstrapper),
		withPubsubConfig(false),
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

	t.RecordMessage("waiting for all nodes to be ready")
	// we are ready; wait for all nodes to be ready
	t.SyncClient.MustBarrier(ctx, stateReady, t.TestInstanceCount)

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

func withPubsubConfig(bootstrapper bool) node.Option {
	return node.Override(new(*config.Pubsub), func() *config.Pubsub {
		return &config.Pubsub{
			Bootstrapper: bootstrapper,
			RemoteTracer: "",
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
