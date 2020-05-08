package chain_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	mocktypes "github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
)

func init() {
	build.InsecurePoStValidation = true
	os.Setenv("TRUST_PARAMS", "1")
	build.SectorSizes = []abi.SectorSize{2048}
	power.ConsensusMinerMinPower = big.NewInt(2048)
}

const source = 0

func (tu *syncTestUtil) repoWithChain(t testing.TB, h int) (repo.Repo, []byte, []*store.FullTipSet) {
	blks := make([]*store.FullTipSet, h)

	for i := 0; i < h; i++ {
		mts, err := tu.g.NextTipSet()
		require.NoError(t, err)

		blks[i] = mts.TipSet
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
		t.Fatalf("%+v", err)
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
	//tu.checkHeight("source", source, h)

	// separate logs
	fmt.Println("\x1b[31m///////////////////////////////////////////////////\x1b[39b")

	return tu
}

func (tu *syncTestUtil) Shutdown() {
	tu.cancel()
}

func (tu *syncTestUtil) printHeads() {
	for i, n := range tu.nds {
		head, err := n.ChainHead(tu.ctx)
		if err != nil {
			tu.t.Fatal(err)
		}

		fmt.Printf("Node %d: %s\n", i, head.Cids())
	}
}

func (tu *syncTestUtil) pushFtsAndWait(to int, fts *store.FullTipSet, wait bool) {
	// TODO: would be great if we could pass a whole tipset here...
	tu.pushTsExpectErr(to, fts, false)

	if wait {
		start := time.Now()
		h, err := tu.nds[to].ChainHead(tu.ctx)
		require.NoError(tu.t, err)
		for !h.Equals(fts.TipSet()) {
			time.Sleep(time.Millisecond * 50)
			h, err = tu.nds[to].ChainHead(tu.ctx)
			require.NoError(tu.t, err)

			if time.Since(start) > time.Second*10 {
				tu.t.Fatal("took too long waiting for block to be accepted")
			}
		}
	}
}

func (tu *syncTestUtil) pushTsExpectErr(to int, fts *store.FullTipSet, experr bool) {
	for _, fb := range fts.Blocks {
		var b types.BlockMsg

		// -1 to match block.Height
		b.Header = fb.Header
		for _, msg := range fb.SecpkMessages {
			c, err := tu.nds[to].(*impl.FullNodeAPI).ChainAPI.Chain.PutMessage(msg)
			require.NoError(tu.t, err)

			b.SecpkMessages = append(b.SecpkMessages, c)
		}

		for _, msg := range fb.BlsMessages {
			c, err := tu.nds[to].(*impl.FullNodeAPI).ChainAPI.Chain.PutMessage(msg)
			require.NoError(tu.t, err)

			b.BlsMessages = append(b.BlsMessages, c)
		}

		err := tu.nds[to].SyncSubmitBlock(tu.ctx, &b)
		if experr {
			require.Error(tu.t, err, "expected submit block to fail")
		} else {
			require.NoError(tu.t, err)
		}
	}
}

func (tu *syncTestUtil) mineOnBlock(blk *store.FullTipSet, src int, miners []int, wait, fail bool) *store.FullTipSet {
	if miners == nil {
		for i := range tu.g.Miners {
			miners = append(miners, i)
		}
	}

	var maddrs []address.Address
	for _, i := range miners {
		maddrs = append(maddrs, tu.g.Miners[i])
	}

	fmt.Println("Miner mining block: ", maddrs)

	mts, err := tu.g.NextTipSetFromMiners(blk.TipSet(), maddrs)
	require.NoError(tu.t, err)

	if fail {
		tu.pushTsExpectErr(src, mts.TipSet, true)
	} else {
		tu.pushFtsAndWait(src, mts.TipSet, wait)
	}

	return mts.TipSet
}

func (tu *syncTestUtil) mineNewBlock(src int, miners []int) {
	mts := tu.mineOnBlock(tu.g.CurTipset, src, miners, true, false)
	tu.g.CurTipset = mts
}

func fblkToBlkMsg(fb *types.FullBlock) *types.BlockMsg {
	out := &types.BlockMsg{
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

	// TODO: Don't ignore stop
	_, err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(sourceRepo),
		node.MockHost(tu.mn),
		node.Test(),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
	)
	require.NoError(tu.t, err)

	lastTs := blocks[len(blocks)-1].Blocks
	for _, lastB := range lastTs {
		cs := out.(*impl.FullNodeAPI).ChainAPI.Chain
		require.NoError(tu.t, cs.AddToTipSetTracker(lastB.Header))
		err = cs.AddBlock(tu.ctx, lastB.Header)
		require.NoError(tu.t, err)
	}

	tu.genesis = genesis
	tu.blocks = blocks
	tu.nds = append(tu.nds, out) // always at 0
}

func (tu *syncTestUtil) addClientNode() int {
	if tu.genesis == nil {
		tu.t.Fatal("source doesn't exists")
	}

	var out api.FullNode

	// TODO: Don't ignore stop
	_, err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(tu.mn),
		node.Test(),

		node.Override(new(modules.Genesis), modules.LoadGenesis(tu.genesis)),
	)
	require.NoError(tu.t, err)

	tu.nds = append(tu.nds, out)
	return len(tu.nds) - 1
}

func (tu *syncTestUtil) pid(n int) peer.ID {
	nal, err := tu.nds[n].NetAddrsListen(tu.ctx)
	require.NoError(tu.t, err)

	return nal.ID
}

func (tu *syncTestUtil) connect(from, to int) {
	toPI, err := tu.nds[to].NetAddrsListen(tu.ctx)
	require.NoError(tu.t, err)

	err = tu.nds[from].NetConnect(tu.ctx, toPI)
	require.NoError(tu.t, err)
}

func (tu *syncTestUtil) disconnect(from, to int) {
	toPI, err := tu.nds[to].NetAddrsListen(tu.ctx)
	require.NoError(tu.t, err)

	err = tu.nds[from].NetDisconnect(tu.ctx, toPI.ID)
	require.NoError(tu.t, err)
}

func (tu *syncTestUtil) checkHeight(name string, n int, h int) {
	b, err := tu.nds[n].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	require.Equal(tu.t, uint64(h), b.Height())
	fmt.Printf("%s H: %d\n", name, b.Height())
}

func (tu *syncTestUtil) compareSourceState(with int) {
	sourceHead, err := tu.nds[source].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	targetHead, err := tu.nds[with].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	if !sourceHead.Equals(targetHead) {
		fmt.Println("different chains: ", sourceHead.Height(), targetHead.Height())
		tu.t.Fatalf("nodes were not synced correctly: %s != %s", sourceHead.Cids(), targetHead.Cids())
	}

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
	target, err := tu.nds[from].ChainHead(tu.ctx)
	if err != nil {
		tu.t.Fatal(err)
	}

	tu.waitUntilSyncTarget(to, target)
}

func (tu *syncTestUtil) waitUntilSyncTarget(to int, target *types.TipSet) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hc, err := tu.nds[to].ChainNotify(ctx)
	if err != nil {
		tu.t.Fatal(err)
	}

	// TODO: some sort of timeout?
	for n := range hc {
		for _, c := range n {
			if c.Val.Equals(target) {
				return
			}
		}
	}
}

func TestSyncSimple(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	//tu.checkHeight("client", client, 0)

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(1, 0)
	tu.waitUntilSync(0, client)

	//tu.checkHeight("client", client, H)

	tu.compareSourceState(client)
}

func TestSyncMining(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	//tu.checkHeight("client", client, 0)

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(client, 0)
	tu.waitUntilSync(0, client)

	//tu.checkHeight("client", client, H)

	tu.compareSourceState(client)

	for i := 0; i < 5; i++ {
		tu.mineNewBlock(0, nil)
		tu.waitUntilSync(0, client)
		tu.compareSourceState(client)
	}
}

func TestSyncBadTimestamp(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(client, 0)
	tu.waitUntilSync(0, client)

	base := tu.g.CurTipset
	tu.g.Timestamper = func(pts *types.TipSet, tl abi.ChainEpoch) uint64 {
		return pts.MinTimestamp() + (build.BlockDelay / 2)
	}

	fmt.Println("BASE: ", base.Cids())
	tu.printHeads()

	a1 := tu.mineOnBlock(base, 0, nil, false, true)

	tu.g.Timestamper = nil
	tu.g.ResyncBankerNonce(a1.TipSet())

	fmt.Println("After mine bad block!")
	tu.printHeads()
	a2 := tu.mineOnBlock(base, 0, nil, true, false)

	tu.waitUntilSync(0, client)

	head, err := tu.nds[0].ChainHead(tu.ctx)
	require.NoError(t, err)

	if !head.Equals(a2.TipSet()) {
		t.Fatalf("expected head to be %s, but got %s", a2.Cids(), head.Cids())
	}
}

func (tu *syncTestUtil) loadChainToNode(to int) {
	// utility to simulate incoming blocks without miner process
	// TODO: should call syncer directly, this won't work correctly in all cases

	for i := 0; i < len(tu.blocks); i++ {
		tu.pushFtsAndWait(to, tu.blocks[i], true)
	}
}

func TestSyncFork(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	p1 := tu.addClientNode()
	p2 := tu.addClientNode()

	fmt.Println("GENESIS: ", tu.g.Genesis().Cid())
	tu.loadChainToNode(p1)
	tu.loadChainToNode(p2)

	phead := func() {
		h1, err := tu.nds[1].ChainHead(tu.ctx)
		require.NoError(tu.t, err)

		h2, err := tu.nds[2].ChainHead(tu.ctx)
		require.NoError(tu.t, err)

		fmt.Println("Node 1: ", h1.Cids(), h1.Parents(), h1.Height())
		fmt.Println("Node 2: ", h2.Cids(), h1.Parents(), h2.Height())
		//time.Sleep(time.Second * 2)
		fmt.Println()
		fmt.Println()
		fmt.Println()
		fmt.Println()
	}

	phead()

	base := tu.g.CurTipset
	fmt.Println("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	// The two nodes fork at this point into 'a' and 'b'
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false)

	tu.g.ResyncBankerNonce(a1.TipSet())
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false)

	fmt.Println("A: ", a.Cids(), a.TipSet().Height())
	fmt.Println("B: ", b.Cids(), b.TipSet().Height())

	// Now for the fun part!!

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilSyncTarget(p1, b.TipSet())
	tu.waitUntilSyncTarget(p2, b.TipSet())

	phead()
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

func TestSyncInputs(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	p1 := tu.addClientNode()

	fn := tu.nds[p1].(*impl.FullNodeAPI)

	s := fn.SyncAPI.Syncer

	err := s.ValidateBlock(context.TODO(), &types.FullBlock{
		Header: &types.BlockHeader{},
	})
	if err == nil {
		t.Fatal("should error on empty block")
	}

	h := mocktypes.MkBlock(nil, 123, 432)

	h.ElectionProof = nil

	err = s.ValidateBlock(context.TODO(), &types.FullBlock{Header: h})
	if err == nil {
		t.Fatal("should error on block with nil election proof")
	}
}
