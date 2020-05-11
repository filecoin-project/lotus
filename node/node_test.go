package node_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/ioutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/storage/mockstorage"
	"github.com/filecoin-project/sector-storage/ffiwrapper"

	"github.com/filecoin-project/go-storedcounter"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	genesis "github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/jsonrpc"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	sectorstorage "github.com/filecoin-project/sector-storage"
	"github.com/filecoin-project/sector-storage/mock"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")

	build.SectorSizes = []abi.SectorSize{2048}
	power.ConsensusMinerMinPower = big.NewInt(2048)
}

func testStorageNode(ctx context.Context, t *testing.T, waddr address.Address, act address.Address, pk crypto.PrivKey, tnd test.TestNode, mn mocknet.Mocknet, opts node.Option) test.TestStorageNode {
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

	ds, err := lr.Datastore("/metadata")
	require.NoError(t, err)
	err = ds.Put(datastore.NewKey("miner-address"), act.Bytes())
	require.NoError(t, err)

	nic := storedcounter.New(ds, datastore.NewKey("/storage/nextid"))
	for i := 0; i < nGenesisPreseals; i++ {
		nic.Next()
	}
	nic.Next()

	err = lr.Close()
	require.NoError(t, err)

	peerid, err := peer.IDFromPrivateKey(pk)
	require.NoError(t, err)

	enc, err := actors.SerializeParams(&saminer.ChangePeerIDParams{NewID: peerid})
	require.NoError(t, err)

	msg := &types.Message{
		To:       act,
		From:     waddr,
		Method:   builtin.MethodsMiner.ChangePeerID,
		Params:   enc,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
	}

	_, err = tnd.MpoolPushMessage(ctx, msg)
	require.NoError(t, err)

	// start node
	var minerapi api.StorageMiner

	mineBlock := make(chan func(bool))
	// TODO: use stop
	_, err = node.New(ctx,
		node.StorageMiner(&minerapi),
		node.Online(),
		node.Repo(r),
		node.Test(),

		node.MockHost(mn),

		node.Override(new(api.FullNode), tnd),
		node.Override(new(*miner.Miner), miner.NewTestMiner(mineBlock, act)),

		opts,
	)
	if err != nil {
		t.Fatalf("failed to construct node: %v", err)
	}

	/*// Bootstrap with full node
	remoteAddrs, err := tnd.NetAddrsListen(ctx)
	require.NoError(t, err)

	err = minerapi.NetConnect(ctx, remoteAddrs)
	require.NoError(t, err)*/
	mineOne := func(ctx context.Context, cb func(bool)) error {
		select {
		case mineBlock <- cb:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return test.TestStorageNode{StorageMiner: minerapi, MineOne: mineOne}
}

func builder(t *testing.T, nFull int, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	ctx := context.Background()
	mn := mocknet.New(ctx)

	fulls := make([]test.TestNode, nFull)
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
		genm, k, err := seed.PreSeal(maddr, abi.RegisteredProof_StackedDRG2KiBPoSt, 0, nGenesisPreseals, tdir, []byte("make genesis mem random"), nil)
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
			Balance: big.NewInt(5000000000000000000),
			Meta:    (&genesis.AccountMeta{Owner: wk.Address}).ActorMeta(),
		})

		keys = append(keys, wk)
		presealDirs = append(presealDirs, tdir)
		maddrs = append(maddrs, maddr)
		genms = append(genms, *genm)
	}

	templ := &genesis.Template{
		Accounts:  genaccs,
		Miners:    genms,
		Timestamp: uint64(time.Now().Unix() - 10000), // some time sufficiently far in the past
	}

	// END PRESEAL SECTION

	for i := 0; i < nFull; i++ {
		var genesis node.Option
		if i == 0 {
			genesis = node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genbuf, *templ))
		} else {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes()))
		}

		var err error
		// TODO: Don't ignore stop
		_, err = node.New(ctx,
			node.FullAPI(&fulls[i].FullNode),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			node.MockHost(mn),
			node.Test(),

			genesis,
		)
		if err != nil {
			t.Fatal(err)
		}

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

		storers[i] = testStorageNode(ctx, t, wa, genMiner, pk, f, mn, node.Options())
		if err := storers[i].StorageAddLocal(ctx, presealDirs[i]); err != nil {
			t.Fatalf("%+v", err)
		}
		/*
			sma := storers[i].StorageMiner.(*impl.StorageMinerAPI)

			psd := presealDirs[i]
		*/
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	return fulls, storers
}

const nGenesisPreseals = 2

func mockSbBuilder(t *testing.T, nFull int, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	ctx := context.Background()
	mn := mocknet.New(ctx)

	fulls := make([]test.TestNode, nFull)
	storers := make([]test.TestStorageNode, len(storage))

	var genbuf bytes.Buffer

	// PRESEAL SECTION, TRY TO REPLACE WITH BETTER IN THE FUTURE
	// TODO: would be great if there was a better way to fake the preseals

	var genms []genesis.Miner
	var genaccs []genesis.Actor
	var maddrs []address.Address
	var presealDirs []string
	var keys []*wallet.Key
	var pidKeys []crypto.PrivKey
	for i := 0; i < len(storage); i++ {
		maddr, err := address.NewIDAddress(genesis2.MinerStart + uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		tdir, err := ioutil.TempDir("", "preseal-memgen")
		if err != nil {
			t.Fatal(err)
		}

		preseals := storage[i].Preseal
		if preseals == test.PresealGenesis {
			preseals = nGenesisPreseals
		}

		genm, k, err := mockstorage.PreSeal(2048, maddr, preseals)
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
			Balance: big.NewInt(5000000000000000000),
			Meta:    (&genesis.AccountMeta{Owner: wk.Address}).ActorMeta(),
		})

		keys = append(keys, wk)
		pidKeys = append(pidKeys, pk)
		presealDirs = append(presealDirs, tdir)
		maddrs = append(maddrs, maddr)
		genms = append(genms, *genm)
	}
	templ := &genesis.Template{
		Accounts:  genaccs,
		Miners:    genms,
		Timestamp: uint64(time.Now().Unix() - 10000),
	}

	// END PRESEAL SECTION

	for i := 0; i < nFull; i++ {
		var genesis node.Option
		if i == 0 {
			genesis = node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genbuf, *templ))
		} else {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes()))
		}

		var err error
		// TODO: Don't ignore stop
		_, err = node.New(ctx,
			node.FullAPI(&fulls[i].FullNode),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			node.MockHost(mn),
			node.Test(),

			node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),

			genesis,
		)
		if err != nil {
			t.Fatalf("%+v", err)
		}
	}

	for i, def := range storage {
		// TODO: support non-bootstrap miners
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

		storers[i] = testStorageNode(ctx, t, genms[i].Worker, maddrs[i], pidKeys[i], f, mn, node.Options(
			node.Override(new(sectorstorage.SectorManager), func() (sectorstorage.SectorManager, error) {
				return mock.NewMockSectorMgr(build.SectorSizes[0]), nil
			}),
			node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
			node.Unset(new(*sectorstorage.Manager)),
		))
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	return fulls, storers
}

func TestAPI(t *testing.T) {
	test.TestApis(t, builder)
}

func rpcBuilder(t *testing.T, nFull int, storage []test.StorageMiner) ([]test.TestNode, []test.TestStorageNode) {
	fullApis, storaApis := builder(t, nFull, storage)
	fulls := make([]test.TestNode, nFull)
	storers := make([]test.TestStorageNode, len(storage))

	for i, a := range fullApis {
		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", a)
		testServ := httptest.NewServer(rpcServer) //  todo: close

		var err error
		fulls[i].FullNode, _, err = client.NewFullNodeRPC("ws://"+testServ.Listener.Addr().String(), nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i, a := range storaApis {
		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", a)
		testServ := httptest.NewServer(rpcServer) //  todo: close

		var err error
		storers[i].StorageMiner, _, err = client.NewStorageMinerRPC("ws://"+testServ.Listener.Addr().String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		storers[i].MineOne = a.MineOne
	}

	return fulls, storers
}

func TestAPIRPC(t *testing.T) {
	test.TestApis(t, rpcBuilder)
}

func TestAPIDealFlow(t *testing.T) {
	logging.SetLogLevel("miner", "ERROR")
	logging.SetLogLevel("chainstore", "ERROR")
	logging.SetLogLevel("chain", "ERROR")
	logging.SetLogLevel("sub", "ERROR")
	logging.SetLogLevel("storageminer", "ERROR")

	t.Run("TestDealFlow", func(t *testing.T) {
		test.TestDealFlow(t, mockSbBuilder, 10*time.Millisecond, false)
	})
	t.Run("WithExportedCAR", func(t *testing.T) {
		test.TestDealFlow(t, mockSbBuilder, 10*time.Millisecond, true)
	})
	t.Run("TestDoubleDealFlow", func(t *testing.T) {
		test.TestDoubleDealFlow(t, mockSbBuilder, 10*time.Millisecond)
	})
}

func TestAPIDealFlowReal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	lotuslog.SetupLogLevels()
	logging.SetLogLevel("miner", "ERROR")
	logging.SetLogLevel("chainstore", "ERROR")
	logging.SetLogLevel("chain", "ERROR")
	logging.SetLogLevel("sub", "ERROR")
	logging.SetLogLevel("storageminer", "ERROR")

	test.TestDealFlow(t, builder, time.Second, false)
}

func TestDealMining(t *testing.T) {
	logging.SetLogLevel("miner", "ERROR")
	logging.SetLogLevel("chainstore", "ERROR")
	logging.SetLogLevel("chain", "ERROR")
	logging.SetLogLevel("sub", "ERROR")
	logging.SetLogLevel("storageminer", "ERROR")

	test.TestDealMining(t, mockSbBuilder, 50*time.Millisecond, false)
}
