package itests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		runAPITest(t, kit.Builder)
	})
	t.Run("rpc", func(t *testing.T) {
		runAPITest(t, kit.RPCBuilder)
	})
}

type apiSuite struct {
	makeNodes kit.APIBuilder
}

// runAPITest is the entry point to API test suite
func runAPITest(t *testing.T, b kit.APIBuilder) {
	ts := apiSuite{
		makeNodes: b,
	}

	t.Run("version", ts.testVersion)
	t.Run("id", ts.testID)
	t.Run("testConnectTwo", ts.testConnectTwo)
	t.Run("testMining", ts.testMining)
	t.Run("testMiningReal", ts.testMiningReal)
	t.Run("testSearchMsg", ts.testSearchMsg)
	t.Run("testNonGenesisMiner", ts.testNonGenesisMiner)
}

func (ts *apiSuite) testVersion(t *testing.T) {
	lapi.RunningNodeType = lapi.NodeFull
	t.Cleanup(func() {
		lapi.RunningNodeType = lapi.NodeUnknown
	})

	ctx := context.Background()
	apis, _ := ts.makeNodes(t, kit.OneFull, kit.OneMiner)
	napi := apis[0]

	v, err := napi.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	versions := strings.Split(v.Version, "+")
	if len(versions) <= 0 {
		t.Fatal("empty version")
	}
	require.Equal(t, versions[0], build.BuildVersion)
}

func (ts *apiSuite) testSearchMsg(t *testing.T) {
	apis, miners := ts.makeNodes(t, kit.OneFull, kit.OneMiner)

	api := apis[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	senderAddr, err := api.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	msg := &types.Message{
		From:  senderAddr,
		To:    senderAddr,
		Value: big.Zero(),
	}
	bm := kit.NewBlockMiner(t, miners[0])
	bm.MineBlocks(ctx, 100*time.Millisecond)
	defer bm.Stop()

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatal("did not successfully send message")
	}

	searchRes, err := api.StateSearchMsg(ctx, types.EmptyTSK, sm.Cid(), lapi.LookbackNoLimit, true)
	if err != nil {
		t.Fatal(err)
	}

	if searchRes.TipSet != res.TipSet {
		t.Fatalf("search ts: %s, different from wait ts: %s", searchRes.TipSet, res.TipSet)
	}

}

func (ts *apiSuite) testID(t *testing.T) {
	ctx := context.Background()
	apis, _ := ts.makeNodes(t, kit.OneFull, kit.OneMiner)
	api := apis[0]

	id, err := api.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assert.Regexp(t, "^12", id.Pretty())
}

func (ts *apiSuite) testConnectTwo(t *testing.T) {
	ctx := context.Background()
	apis, _ := ts.makeNodes(t, kit.TwoFull, kit.OneMiner)

	p, err := apis[0].NetPeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 0 {
		t.Error("Node 0 has a peer")
	}

	p, err = apis[1].NetPeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 0 {
		t.Error("Node 1 has a peer")
	}

	addrs, err := apis[1].NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := apis[0].NetConnect(ctx, addrs); err != nil {
		t.Fatal(err)
	}

	p, err = apis[0].NetPeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 {
		t.Error("Node 0 doesn't have 1 peer")
	}

	p, err = apis[1].NetPeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 {
		t.Error("Node 0 doesn't have 1 peer")
	}
}

func (ts *apiSuite) testMining(t *testing.T) {
	ctx := context.Background()
	fulls, miners := ts.makeNodes(t, kit.OneFull, kit.OneMiner)
	api := fulls[0]

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	initHead := (<-newHeads)[0]
	baseHeight := initHead.Val.Height()

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(h1.Height()), int64(baseHeight))

	bm := kit.NewBlockMiner(t, miners[0])
	bm.MineUntilBlock(ctx, fulls[0], nil)
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(h2.Height()), int64(h1.Height()))
}

func (ts *apiSuite) testMiningReal(t *testing.T) {
	build.InsecurePoStValidation = false
	defer func() {
		build.InsecurePoStValidation = true
	}()

	ctx := context.Background()
	fulls, miners := ts.makeNodes(t, kit.OneFull, kit.OneMiner)
	api := fulls[0]

	newHeads, err := api.ChainNotify(ctx)
	require.NoError(t, err)
	at := (<-newHeads)[0].Val.Height()

	h1, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(at), int64(h1.Height()))

	bm := kit.NewBlockMiner(t, miners[0])

	bm.MineUntilBlock(ctx, fulls[0], nil)
	require.NoError(t, err)

	<-newHeads

	h2, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(h2.Height()), int64(h1.Height()))

	bm.MineUntilBlock(ctx, fulls[0], nil)
	require.NoError(t, err)

	<-newHeads

	h3, err := api.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(h3.Height()), int64(h2.Height()))
}

func (ts *apiSuite) testNonGenesisMiner(t *testing.T) {
	ctx := context.Background()
	n, sn := ts.makeNodes(t,
		[]kit.FullNodeOpts{kit.FullNodeWithLatestActorsAt(-1)},
		[]kit.StorageMiner{{Full: 0, Preseal: kit.PresealGenesis}},
	)

	full, ok := n[0].FullNode.(*impl.FullNodeAPI)
	if !ok {
		t.Skip("not testing with a full node")
		return
	}
	genesisMiner := sn[0]

	bm := kit.NewBlockMiner(t, genesisMiner)
	bm.MineBlocks(ctx, 4*time.Millisecond)
	t.Cleanup(bm.Stop)

	gaa, err := genesisMiner.ActorAddress(ctx)
	require.NoError(t, err)

	gmi, err := full.StateMinerInfo(ctx, gaa, types.EmptyTSK)
	require.NoError(t, err)

	testm := n[0].Stb(ctx, t, kit.TestSpt, gmi.Owner)

	ta, err := testm.ActorAddress(ctx)
	require.NoError(t, err)

	tid, err := address.IDFromAddress(ta)
	require.NoError(t, err)

	require.Equal(t, uint64(1001), tid)
}
