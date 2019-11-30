package node_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/ioutil"
	"net/http/httptest"
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/lib/jsonrpc"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
)

func testStorageNode(ctx context.Context, t *testing.T, waddr address.Address, act address.Address, pk crypto.PrivKey, tnd test.TestNode, mn mocknet.Mocknet) test.TestStorageNode {
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

	err = lr.Close()
	require.NoError(t, err)

	peerid, err := peer.IDFromPrivateKey(pk)
	require.NoError(t, err)

	enc, err := actors.SerializeParams(&actors.UpdatePeerIDParams{PeerID: peerid})
	require.NoError(t, err)

	msg := &types.Message{
		To:       act,
		From:     waddr,
		Method:   actors.MAMethods.UpdatePeerID,
		Params:   enc,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
	}

	_, err = tnd.MpoolPushMessage(ctx, msg)
	require.NoError(t, err)

	// start node
	var minerapi api.StorageMiner

	// TODO: use stop
	_, err = node.New(ctx,
		node.StorageMiner(&minerapi),
		node.Online(),
		node.Repo(r),
		node.Test(),

		node.MockHost(mn),

		node.Override(new(api.FullNode), tnd),
	)
	if err != nil {
		t.Fatalf("failed to construct node: %v", err)
	}

	/*// Bootstrap with full node
	remoteAddrs, err := tnd.NetAddrsListen(ctx)
	require.NoError(t, err)

	err = minerapi.NetConnect(ctx, remoteAddrs)
	require.NoError(t, err)*/

	return test.TestStorageNode{minerapi}
}

func builder(t *testing.T, nFull int, storage []int) ([]test.TestNode, []test.TestStorageNode) {
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
	gmc := &gen.GenMinerCfg{
		PeerIDs:  []peer.ID{minerPid}, // TODO: if we have more miners, need more peer IDs
		PreSeals: map[string]genesis.GenesisMiner{},
	}
	for i := 0; i < len(storage); i++ {
		maddr, err := address.NewIDAddress(300 + uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		tdir, err := ioutil.TempDir("", "preseal-memgen")
		if err != nil {
			t.Fatal(err)
		}
		genm, err := seed.PreSeal(maddr, 1024, 1, tdir, []byte("make genesis mem random"))
		if err != nil {
			t.Fatal(err)
		}
		gmc.MinerAddrs = append(gmc.MinerAddrs, maddr)
		gmc.PreSeals[maddr.String()] = *genm
	}

	// END PRESEAL SECTION

	for i := 0; i < nFull; i++ {
		var genesis node.Option
		if i == 0 {
			genesis = node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genbuf, gmc))
		} else {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes()))
		}

		mineBlock := make(chan struct{})

		var err error
		// TODO: Don't ignore stop
		_, err = node.New(ctx,
			node.FullAPI(&fulls[i].FullNode),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			node.MockHost(mn),
			node.Test(),

			node.Override(new(*miner.Miner), miner.NewTestMiner(mineBlock)),

			genesis,
		)
		if err != nil {
			t.Fatal(err)
		}

		fulls[i].MineOne = func(ctx context.Context) error {
			select {
			case mineBlock <- struct{}{}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	for i, full := range storage {
		// TODO: support non-bootstrap miners
		if i != 0 {
			t.Fatal("only one storage node supported")
		}
		if full != 0 {
			t.Fatal("storage nodes only supported on the first full node")
		}

		f := fulls[full]

		wa, err := f.WalletDefaultAddress(ctx)
		require.NoError(t, err)

		genMiner, err := address.NewFromString("t0102")
		require.NoError(t, err)

		storers[i] = testStorageNode(ctx, t, wa, genMiner, pk, f, mn)
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	return fulls, storers
}

func TestAPI(t *testing.T) {
	test.TestApis(t, builder)
}

func rpcBuilder(t *testing.T, nFull int, storage []int) ([]test.TestNode, []test.TestStorageNode) {
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
		fulls[i].MineOne = a.MineOne
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
	}

	return fulls, storers
}

func TestAPIRPC(t *testing.T) {
	test.TestApis(t, rpcBuilder)
}

func TestAPIDealFlow(t *testing.T) {
	test.TestDealFlow(t, builder)
}
