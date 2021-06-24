package test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/ioutil"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/gen"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/mock"
	"github.com/filecoin-project/lotus/genesis"
	lotusminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	testing2 "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/mockstorage"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func init() {
	chain.BootstrapPeerThreshold = 1
	messagepool.HeadChangeCoalesceMinDelay = time.Microsecond
	messagepool.HeadChangeCoalesceMaxDelay = 2 * time.Microsecond
	messagepool.HeadChangeCoalesceMergeInterval = 100 * time.Nanosecond
}

func CreateTestStorageNode(ctx context.Context, t *testing.T, waddr address.Address, act address.Address, pk crypto.PrivKey, tnd test.TestNode, mn mocknet.Mocknet, opts node.Option) test.TestStorageNode {
	r := repo.NewMemory(nil)

	lr, err := r.Lock(repo.StorageMiner)
	require.NoError(t, err)

	ks, err := lr.KeyStore()
	require.NoError(t, err)

	kbytes, err := pk.Bytes()
	require.NoError(t, err)

	err = ks.Put("libp2p-host", types.KeyInfo{
		Type:       "libp2p-host",
		PrivateKey: kbytes,
	})
	require.NoError(t, err)

	ds, err := lr.Datastore(context.TODO(), "/metadata")
	require.NoError(t, err)
	err = ds.Put(datastore.NewKey("miner-address"), act.Bytes())
	require.NoError(t, err)

	nic := storedcounter.New(ds, datastore.NewKey(modules.StorageCounterDSPrefix))
	for i := 0; i < test.GenesisPreseals; i++ {
		_, err := nic.Next()
		require.NoError(t, err)
	}
	_, err = nic.Next()
	require.NoError(t, err)

	err = lr.Close()
	require.NoError(t, err)

	peerid, err := peer.IDFromPrivateKey(pk)
	require.NoError(t, err)

	enc, err := actors.SerializeParams(&miner2.ChangePeerIDParams{NewID: abi.PeerID(peerid)})
	require.NoError(t, err)

	msg := &types.Message{
		To:     act,
		From:   waddr,
		Method: miner.Methods.ChangePeerID,
		Params: enc,
		Value:  types.NewInt(0),
	}

	_, err = tnd.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	// start node
	var minerapi api.StorageMiner

	mineBlock := make(chan lotusminer.MineReq)
	stop, err := node.New(ctx,
		node.StorageMiner(&minerapi),
		node.Online(),
		node.Repo(r),
		node.Test(),

		node.MockHost(mn),

		node.Override(new(v1api.FullNode), tnd),
		node.Override(new(*lotusminer.Miner), lotusminer.NewTestMiner(mineBlock, act)),

		opts,
	)
	if err != nil {
		t.Fatalf("failed to construct node: %v", err)
	}

	t.Cleanup(func() { _ = stop(context.Background()) })

	/*// Bootstrap with full node
	remoteAddrs, err := tnd.NetAddrsListen(ctx)
	require.NoError(t, err)

	err = minerapi.NetConnect(ctx, remoteAddrs)
	require.NoError(t, err)*/
	mineOne := func(ctx context.Context, req lotusminer.MineReq) error {
		select {
		case mineBlock <- req:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return test.TestStorageNode{StorageMiner: minerapi, MineOne: mineOne, Stop: stop}
}

func storageBuilder(parentNode test.TestNode, mn mocknet.Mocknet, opts node.Option) test.StorageBuilder {
	return func(ctx context.Context, t *testing.T, spt abi.RegisteredSealProof, owner address.Address) test.TestStorageNode {
		pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
		require.NoError(t, err)

		minerPid, err := peer.IDFromPrivateKey(pk)
		require.NoError(t, err)

		params, serr := actors.SerializeParams(&power2.CreateMinerParams{
			Owner:         owner,
			Worker:        owner,
			SealProofType: spt,
			Peer:          abi.PeerID(minerPid),
		})
		require.NoError(t, serr)

		createStorageMinerMsg := &types.Message{
			To:    power.Address,
			From:  owner,
			Value: big.Zero(),

			Method: power.Methods.CreateMiner,
			Params: params,

			GasLimit:   0,
			GasPremium: big.NewInt(5252),
		}

		signed, err := parentNode.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
		require.NoError(t, err)

		mw, err := parentNode.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, mw.Receipt.ExitCode)

		var retval power2.CreateMinerReturn
		err = retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return))
		require.NoError(t, err)

		return CreateTestStorageNode(ctx, t, owner, retval.IDAddress, pk, parentNode, mn, opts)
	}
}

func Builder(t *testing.T, fullOpts []test.FullNodeOpts, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	return mockBuilderOpts(t, fullOpts, storage, false)
}

func MockSbBuilder(t *testing.T, fullOpts []test.FullNodeOpts, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	return mockSbBuilderOpts(t, fullOpts, storage, false)
}

func RPCBuilder(t *testing.T, fullOpts []test.FullNodeOpts, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	return mockBuilderOpts(t, fullOpts, storage, true)
}

func RPCMockSbBuilder(t *testing.T, fullOpts []test.FullNodeOpts, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	return mockSbBuilderOpts(t, fullOpts, storage, true)
}

func mockBuilderOpts(t *testing.T, fullOpts []test.FullNodeOpts, storage []test.StorageMiner, rpc bool) ([]test.TestNode, []test.TestStorageNode) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mn := mocknet.New(ctx)

	fulls := make([]test.TestNode, len(fullOpts))
	storers := make([]test.TestStorageNode, len(storage))

	pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)

	minerPid, err := peer.IDFromPrivateKey(pk)
	require.NoError(t, err)

	var genbuf bytes.Buffer

	if len(storage) > 1 {
		panic("need more peer IDs")
	}
	// PRESEAL SECTION, TRY TO REPLACE WITH BETTER IN THE FUTURE
	// TODO: would be great if there was a better way to fake the preseals

	var genms []genesis.Miner
	var maddrs []address.Address
	var genaccs []genesis.Actor
	var keys []*wallet.Key

	var presealDirs []string
	for i := 0; i < len(storage); i++ {
		maddr, err := address.NewIDAddress(genesis2.MinerStart + uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		tdir, err := ioutil.TempDir("", "preseal-memgen")
		if err != nil {
			t.Fatal(err)
		}
		genm, k, err := seed.PreSeal(maddr, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, test.GenesisPreseals, tdir, []byte("make genesis mem random"), nil, true)
		if err != nil {
			t.Fatal(err)
		}
		genm.PeerId = minerPid

		wk, err := wallet.NewKey(*k)
		if err != nil {
			return nil, nil
		}

		genaccs = append(genaccs, genesis.Actor{
			Type:    genesis.TAccount,
			Balance: big.Mul(big.NewInt(400000000), types.NewInt(build.FilecoinPrecision)),
			Meta:    (&genesis.AccountMeta{Owner: wk.Address}).ActorMeta(),
		})

		keys = append(keys, wk)
		presealDirs = append(presealDirs, tdir)
		maddrs = append(maddrs, maddr)
		genms = append(genms, *genm)
	}
	templ := &genesis.Template{
		Accounts:         genaccs,
		Miners:           genms,
		NetworkName:      "test",
		Timestamp:        uint64(time.Now().Unix() - 10000), // some time sufficiently far in the past
		VerifregRootKey:  gen.DefaultVerifregRootkeyActor,
		RemainderAccount: gen.DefaultRemainderAccountActor,
	}

	// END PRESEAL SECTION

	for i := 0; i < len(fullOpts); i++ {
		var genesis node.Option
		if i == 0 {
			genesis = node.Override(new(modules.Genesis), testing2.MakeGenesisMem(&genbuf, *templ))
		} else {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes()))
		}

		stop, err := node.New(ctx,
			node.FullAPI(&fulls[i].FullNode, node.Lite(fullOpts[i].Lite)),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			node.MockHost(mn),
			node.Test(),

			genesis,

			fullOpts[i].Opts(fulls),
		)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = stop(context.Background()) })

		if rpc {
			fulls[i] = fullRpc(t, fulls[i])
		}

		fulls[i].Stb = storageBuilder(fulls[i], mn, node.Options())
	}

	for i, def := range storage {
		// TODO: support non-bootstrap miners
		if i != 0 {
			t.Fatal("only one storage node supported")
		}
		if def.Full != 0 {
			t.Fatal("storage nodes only supported on the first full node")
		}

		f := fulls[def.Full]
		if _, err := f.FullNode.WalletImport(ctx, &keys[i].KeyInfo); err != nil {
			t.Fatal(err)
		}
		if err := f.FullNode.WalletSetDefault(ctx, keys[i].Address); err != nil {
			t.Fatal(err)
		}

		genMiner := maddrs[i]
		wa := genms[i].Worker

		opts := def.Opts
		if opts == nil {
			opts = node.Options()
		}
		storers[i] = CreateTestStorageNode(ctx, t, wa, genMiner, pk, f, mn, opts)
		if err := storers[i].StorageAddLocal(ctx, presealDirs[i]); err != nil {
			t.Fatalf("%+v", err)
		}
		/*
			sma := storers[i].StorageMiner.(*impl.StorageMinerAPI)

			psd := presealDirs[i]
		*/
		if rpc {
			storers[i] = storerRpc(t, storers[i])
		}
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	if len(storers) > 0 {
		// Mine 2 blocks to setup some CE stuff in some actors
		var wait sync.Mutex
		wait.Lock()

		test.MineUntilBlock(ctx, t, fulls[0], storers[0], func(epoch abi.ChainEpoch) {
			wait.Unlock()
		})

		wait.Lock()
		test.MineUntilBlock(ctx, t, fulls[0], storers[0], func(epoch abi.ChainEpoch) {
			wait.Unlock()
		})
		wait.Lock()
	}

	return fulls, storers
}

func mockSbBuilderOpts(t *testing.T, fullOpts []test.FullNodeOpts, storage []test.StorageMiner, rpc bool) ([]test.TestNode, []test.TestStorageNode) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mn := mocknet.New(ctx)

	fulls := make([]test.TestNode, len(fullOpts))
	storers := make([]test.TestStorageNode, len(storage))

	var genbuf bytes.Buffer

	// PRESEAL SECTION, TRY TO REPLACE WITH BETTER IN THE FUTURE
	// TODO: would be great if there was a better way to fake the preseals

	var genms []genesis.Miner
	var genaccs []genesis.Actor
	var maddrs []address.Address
	var keys []*wallet.Key
	var pidKeys []crypto.PrivKey
	for i := 0; i < len(storage); i++ {
		maddr, err := address.NewIDAddress(genesis2.MinerStart + uint64(i))
		if err != nil {
			t.Fatal(err)
		}

		preseals := storage[i].Preseal
		if preseals == test.PresealGenesis {
			preseals = test.GenesisPreseals
		}

		genm, k, err := mockstorage.PreSeal(abi.RegisteredSealProof_StackedDrg2KiBV1, maddr, preseals)
		if err != nil {
			t.Fatal(err)
		}

		pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
		require.NoError(t, err)

		minerPid, err := peer.IDFromPrivateKey(pk)
		require.NoError(t, err)

		genm.PeerId = minerPid

		wk, err := wallet.NewKey(*k)
		if err != nil {
			return nil, nil
		}

		genaccs = append(genaccs, genesis.Actor{
			Type:    genesis.TAccount,
			Balance: big.Mul(big.NewInt(400000000), types.NewInt(build.FilecoinPrecision)),
			Meta:    (&genesis.AccountMeta{Owner: wk.Address}).ActorMeta(),
		})

		keys = append(keys, wk)
		pidKeys = append(pidKeys, pk)
		maddrs = append(maddrs, maddr)
		genms = append(genms, *genm)
	}
	templ := &genesis.Template{
		Accounts:         genaccs,
		Miners:           genms,
		NetworkName:      "test",
		Timestamp:        uint64(time.Now().Unix()) - (build.BlockDelaySecs * 20000),
		VerifregRootKey:  gen.DefaultVerifregRootkeyActor,
		RemainderAccount: gen.DefaultRemainderAccountActor,
	}

	// END PRESEAL SECTION

	for i := 0; i < len(fullOpts); i++ {
		var genesis node.Option
		if i == 0 {
			genesis = node.Override(new(modules.Genesis), testing2.MakeGenesisMem(&genbuf, *templ))
		} else {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes()))
		}

		stop, err := node.New(ctx,
			node.FullAPI(&fulls[i].FullNode, node.Lite(fullOpts[i].Lite)),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			node.MockHost(mn),
			node.Test(),

			node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
			node.Override(new(ffiwrapper.Prover), mock.MockProver),

			// so that we subscribe to pubsub topics immediately
			node.Override(new(dtypes.Bootstrapper), dtypes.Bootstrapper(true)),

			genesis,

			fullOpts[i].Opts(fulls),
		)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		t.Cleanup(func() { _ = stop(context.Background()) })

		if rpc {
			fulls[i] = fullRpc(t, fulls[i])
		}

		fulls[i].Stb = storageBuilder(fulls[i], mn, node.Options(
			node.Override(new(sectorstorage.SectorManager), func() (sectorstorage.SectorManager, error) {
				return mock.NewMockSectorMgr(nil), nil
			}),
			node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
			node.Override(new(ffiwrapper.Prover), mock.MockProver),
			node.Unset(new(*sectorstorage.Manager)),
		))
	}

	for i, def := range storage {
		// TODO: support non-bootstrap miners

		minerID := abi.ActorID(genesis2.MinerStart + uint64(i))

		if def.Full != 0 {
			t.Fatal("storage nodes only supported on the first full node")
		}

		f := fulls[def.Full]
		if _, err := f.FullNode.WalletImport(ctx, &keys[i].KeyInfo); err != nil {
			return nil, nil
		}
		if err := f.FullNode.WalletSetDefault(ctx, keys[i].Address); err != nil {
			return nil, nil
		}

		sectors := make([]abi.SectorID, len(genms[i].Sectors))
		for i, sector := range genms[i].Sectors {
			sectors[i] = abi.SectorID{
				Miner:  minerID,
				Number: sector.SectorID,
			}
		}

		opts := def.Opts
		if opts == nil {
			opts = node.Options()
		}
		storers[i] = CreateTestStorageNode(ctx, t, genms[i].Worker, maddrs[i], pidKeys[i], f, mn, node.Options(
			node.Override(new(sectorstorage.SectorManager), func() (sectorstorage.SectorManager, error) {
				return mock.NewMockSectorMgr(sectors), nil
			}),
			node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
			node.Override(new(ffiwrapper.Prover), mock.MockProver),
			node.Unset(new(*sectorstorage.Manager)),
			opts,
		))

		if rpc {
			storers[i] = storerRpc(t, storers[i])
		}
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	if len(storers) > 0 {
		// Mine 2 blocks to setup some CE stuff in some actors
		var wait sync.Mutex
		wait.Lock()

		test.MineUntilBlock(ctx, t, fulls[0], storers[0], func(abi.ChainEpoch) {
			wait.Unlock()
		})
		wait.Lock()
		test.MineUntilBlock(ctx, t, fulls[0], storers[0], func(abi.ChainEpoch) {
			wait.Unlock()
		})
		wait.Lock()
	}

	return fulls, storers
}

func fullRpc(t *testing.T, nd test.TestNode) test.TestNode {
	ma, listenAddr, err := CreateRPCServer(t, map[string]interface{}{
		"/rpc/v1": nd,
		"/rpc/v0": &v0api.WrapperV1Full{FullNode: nd},
	})
	require.NoError(t, err)

	var stop func()
	var full test.TestNode
	full.FullNode, stop, err = client.NewFullNodeRPCV1(context.Background(), listenAddr+"/rpc/v1", nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	full.ListenAddr = ma
	return full
}

func storerRpc(t *testing.T, nd test.TestStorageNode) test.TestStorageNode {
	ma, listenAddr, err := CreateRPCServer(t, map[string]interface{}{
		"/rpc/v0": nd,
	})
	require.NoError(t, err)

	var stop func()
	var storer test.TestStorageNode
	storer.StorageMiner, stop, err = client.NewStorageMinerRPCV0(context.Background(), listenAddr+"/rpc/v0", nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	storer.ListenAddr = ma
	storer.MineOne = nd.MineOne
	return storer
}

func CreateRPCServer(t *testing.T, handlers map[string]interface{}) (multiaddr.Multiaddr, string, error) {
	m := mux.NewRouter()
	for path, handler := range handlers {
		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", handler)
		m.Handle(path, rpcServer)
	}
	testServ := httptest.NewServer(m) //  todo: close
	t.Cleanup(testServ.Close)
	t.Cleanup(testServ.CloseClientConnections)

	addr := testServ.Listener.Addr()
	listenAddr := "ws://" + addr.String()
	ma, err := parseWSMultiAddr(addr)
	if err != nil {
		return nil, "", err
	}
	return ma, listenAddr, err
}

func parseWSMultiAddr(addr net.Addr) (multiaddr.Multiaddr, error) {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	ma, err := multiaddr.NewMultiaddr("/ip4/" + host + "/" + addr.Network() + "/" + port + "/ws")
	if err != nil {
		return nil, err
	}
	return ma, nil
}

func WSMultiAddrToString(addr multiaddr.Multiaddr) (string, error) {
	parts := strings.Split(addr.String(), "/")
	if len(parts) != 6 || parts[0] != "" {
		return "", xerrors.Errorf("Malformed ws multiaddr %s", addr)
	}

	host := parts[2]
	port := parts[4]
	proto := parts[5]

	return proto + "://" + host + ":" + port + "/rpc/v0", nil
}
