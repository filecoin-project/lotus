package chain_test

import (
	"context"
	"fmt"
	"testing"

	logging "github.com/ipfs/go-log"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/repo"
)

const source = 0

func (tu *syncTestUtil) repoWithChain(t testing.TB, h int) (repo.Repo, []byte, []*store.FullTipSet) {
	blks := make([]*store.FullTipSet, h)

	for i := 0; i < h; i++ {
		mts, err := tu.g.NextTipSet()
		require.NoError(t, err)

		blks[i] = mts.TipSet

		ts := mts.TipSet.TipSet()
		fmt.Printf("tipset at H:%d: %s\n", ts.Height(), ts.Cids())

		require.Equal(t, uint64(i+1), ts.Height(), "wrong height")
	}

	r, err := tu.g.YieldRepo()
	require.NoError(t, err)

	genb, err := tu.g.GenesisCar()
	require.NoError(t, err)

	return r, genb, blks
}

type syncTestUtil struct {
	t testing.TB

	ctx    context.Context
	cancel func()

	mn mocknet.Mocknet

	g *gen.ChainGen

	genesis []byte
	blocks  []*store.FullTipSet

	nds []api.FullNode
}

func prepSyncTest(t testing.TB, h int) *syncTestUtil {
	logging.SetLogLevel("*", "INFO")

	g, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tu := &syncTestUtil{
		t:      t,
		ctx:    ctx,
		cancel: cancel,

		mn: mocknet.New(ctx),
		g:  g,
	}

	tu.addSourceNode(h)
	tu.checkHeight("source", source, h)

	// separate logs
	fmt.Println("\x1b[31m///////////////////////////////////////////////////\x1b[39b")

	return tu
}

func (tu *syncTestUtil) Shutdown() {
	tu.cancel()
}

func (tu *syncTestUtil) mineNewBlock(src int) {
	mts, err := tu.g.NextTipSet()
	require.NoError(tu.t, err)

	for _, msg := range mts.Messages {
		require.NoError(tu.t, tu.nds[src].MpoolPush(context.TODO(), msg))
	}

	for _, fblk := range mts.TipSet.Blocks {
		require.NoError(tu.t, tu.nds[src].ChainSubmitBlock(context.TODO(), fblkToBlkMsg(fblk)))
	}
}

func fblkToBlkMsg(fb *types.FullBlock) *chain.BlockMsg {
	out := &chain.BlockMsg{
		Header: fb.Header,
	}

	for _, msg := range fb.BlsMessages {
		out.BlsMessages = append(out.BlsMessages, msg.Cid())
	}
	for _, msg := range fb.SecpkMessages {
		out.SecpkMessages = append(out.SecpkMessages, msg.Cid())
	}
	return out
}

func (tu *syncTestUtil) addSourceNode(gen int) {
	if tu.genesis != nil {
		tu.t.Fatal("source node already exists")
	}

	sourceRepo, genesis, blocks := tu.repoWithChain(tu.t, gen)
	var out api.FullNode

	err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(sourceRepo),
		node.MockHost(tu.mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
	)
	require.NoError(tu.t, err)

	tu.genesis = genesis
	tu.blocks = blocks
	tu.nds = append(tu.nds, out) // always at 0
}

func (tu *syncTestUtil) addClientNode() int {
	if tu.genesis == nil {
		tu.t.Fatal("source doesn't exists")
	}

	var out api.FullNode

	err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(tu.mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(tu.genesis)),
	)
	require.NoError(tu.t, err)

	tu.nds = append(tu.nds, out)
	return len(tu.nds) - 1
}

func (tu *syncTestUtil) connect(from, to int) {
	toPI, err := tu.nds[to].NetAddrsListen(tu.ctx)
	require.NoError(tu.t, err)

	err = tu.nds[from].NetConnect(tu.ctx, toPI)
	require.NoError(tu.t, err)
}

func (tu *syncTestUtil) checkHeight(name string, n int, h int) {
	b, err := tu.nds[n].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	require.Equal(tu.t, uint64(h), b.Height())
	fmt.Printf("%s H: %d\n", name, b.Height())
}

func (tu *syncTestUtil) compareSourceState(with int) {
	sourceAccounts, err := tu.nds[source].WalletList(tu.ctx)
	require.NoError(tu.t, err)

	for _, addr := range sourceAccounts {
		sourceBalance, err := tu.nds[source].WalletBalance(tu.ctx, addr)
		require.NoError(tu.t, err)
		fmt.Printf("Source state check for %s, expect %s\n", addr, sourceBalance)

		actBalance, err := tu.nds[with].WalletBalance(tu.ctx, addr)
		require.NoError(tu.t, err)

		require.Equal(tu.t, sourceBalance, actBalance)
		fmt.Printf("Source state check <OK> for %s\n", addr)
	}
}

func (tu *syncTestUtil) waitUntilSync(from, to int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target, err := tu.nds[from].ChainHead(ctx)
	if err != nil {
		tu.t.Fatal(err)
	}

	hc, err := tu.nds[to].ChainNotify(ctx)
	if err != nil {
		tu.t.Fatal(err)
	}

	// TODO: some sort of timeout?
	for c := range hc {
		if c.Val.Equals(target) {
			return
		}
	}
}

/*
func (tu *syncTestUtil) submitSourceBlock(to int, h int) {
	// utility to simulate incoming blocks without miner process
	// TODO: should call syncer directly, this won't work correctly in all cases

	var b chain.BlockMsg

	// -1 to match block.Height
	b.Header = tu.blocks[h-1].Header
	for _, msg := range tu.blocks[h-1].SecpkMessages {
		c, err := tu.nds[to].(*impl.FullNodeAPI).ChainAPI.Chain.PutMessage(msg)
		require.NoError(tu.t, err)

		b.SecpkMessages = append(b.SecpkMessages, c)
	}

	require.NoError(tu.t, tu.nds[to].ChainSubmitBlock(tu.ctx, &b))
}

func (tu *syncTestUtil) submitSourceBlocks(to int, h int, n int) {
	for i := 0; i < n; i++ {
		tu.submitSourceBlock(to, h+i)
	}
}
*/

func TestSyncSimple(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	tu.checkHeight("client", client, 0)

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(1, 0)
	tu.waitUntilSync(0, client)

	tu.checkHeight("client", client, H)

	tu.compareSourceState(client)
}

func TestSyncMining(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	tu.checkHeight("client", client, 0)

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(client, 0)
	fmt.Println("waiting for sync...")
	tu.waitUntilSync(0, client)

	fmt.Println("after wait until sync")

	tu.checkHeight("client", client, H)

	tu.compareSourceState(client)

	for i := 0; i < 5; i++ {
		tu.mineNewBlock(0)
		tu.waitUntilSync(0, client)
		tu.compareSourceState(client)
	}
}

func BenchmarkSyncBasic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runSyncBenchLength(b, 100)
	}
}

func runSyncBenchLength(b *testing.B, l int) {
	tu := prepSyncTest(b, l)

	client := tu.addClientNode()
	tu.checkHeight("client", client, 0)

	b.ResetTimer()

	require.NoError(b, tu.mn.LinkAll())
	tu.connect(1, 0)

	tu.waitUntilSync(0, client)
}

/*
TODO: this is broken because of how tu.submitSourceBlock works now
func TestSyncManual(t *testing.T) {
	H := 20
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	tu.checkHeight("client", client, 0)

	tu.submitSourceBlocks(client, 1, H)

	time.Sleep(time.Second * 1)

	tu.checkHeight("client", client, H)

	tu.compareSourceState(client)
}

func TestSyncIncoming(t *testing.T) {
	H := 1
	tu := prepSyncTest(t, H)

	producer := tu.addClientNode()
	client := tu.addClientNode()

	tu.mn.LinkAll()
	tu.connect(client, producer)

	for h := 0; h < H; h++ {
		tu.submitSourceBlock(producer, h + 1)

		time.Sleep(time.Millisecond * 200)

	}
	tu.checkHeight("client", client, H)
	tu.checkHeight("producer", producer, H)

	tu.compareSourceState(client)
}
*/
