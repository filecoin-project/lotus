package chain_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/specs-actors/actors/runtime/proof"

	"github.com/ipfs/go-cid"

	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
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
	err := os.Setenv("TRUST_PARAMS", "1")
	if err != nil {
		panic(err)
	}
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
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

func (tu *syncTestUtil) mineOnBlock(blk *store.FullTipSet, to int, miners []int, wait, fail bool, msgs [][]*types.SignedMessage) *store.FullTipSet {
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

	var nts *store.FullTipSet
	var err error
	if msgs != nil {
		nts, err = tu.g.NextTipSetFromMinersWithMessages(blk.TipSet(), maddrs, msgs)
		require.NoError(tu.t, err)
	} else {
		mt, err := tu.g.NextTipSetFromMiners(blk.TipSet(), maddrs)
		require.NoError(tu.t, err)
		nts = mt.TipSet
	}

	if fail {
		tu.pushTsExpectErr(to, nts, true)
	} else {
		tu.pushFtsAndWait(to, nts, wait)
	}

	return nts
}

func (tu *syncTestUtil) mineNewBlock(src int, miners []int) {
	mts := tu.mineOnBlock(tu.g.CurTipset, src, miners, true, false, nil)
	tu.g.CurTipset = mts
}

func (tu *syncTestUtil) addSourceNode(gen int) {
	if tu.genesis != nil {
		tu.t.Fatal("source node already exists")
	}

	sourceRepo, genesis, blocks := tu.repoWithChain(tu.t, gen)
	var out api.FullNode

	stop, err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(sourceRepo),
		node.MockHost(tu.mn),
		node.Test(),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
	)
	require.NoError(tu.t, err)
	tu.t.Cleanup(func() { _ = stop(context.Background()) })

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

	stop, err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(tu.mn),
		node.Test(),

		node.Override(new(modules.Genesis), modules.LoadGenesis(tu.genesis)),
	)
	require.NoError(tu.t, err)
	tu.t.Cleanup(func() { _ = stop(context.Background()) })

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

func (tu *syncTestUtil) assertBad(node int, ts *types.TipSet) {
	for _, blk := range ts.Cids() {
		rsn, err := tu.nds[node].SyncCheckBad(context.TODO(), blk)
		require.NoError(tu.t, err)
		require.True(tu.t, len(rsn) != 0)
	}
}

func (tu *syncTestUtil) getHead(node int) *types.TipSet {
	ts, err := tu.nds[node].ChainHead(context.TODO())
	require.NoError(tu.t, err)
	return ts
}

func (tu *syncTestUtil) checkpointTs(node int, tsk types.TipSetKey) {
	require.NoError(tu.t, tu.nds[node].SyncCheckpoint(context.TODO(), tsk))
}

func (tu *syncTestUtil) waitUntilNodeHasTs(node int, tsk types.TipSetKey) {
	for {
		_, err := tu.nds[node].ChainGetTipSet(context.TODO(), tsk)
		if err != nil {
			break
		}
	}

	// Time to allow for syncing and validation
	time.Sleep(2 * time.Second)
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
		return pts.MinTimestamp() + (build.BlockDelaySecs / 2)
	}

	fmt.Println("BASE: ", base.Cids())
	tu.printHeads()

	a1 := tu.mineOnBlock(base, 0, nil, false, true, nil)

	tu.g.Timestamper = nil
	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))

	tu.nds[0].(*impl.FullNodeAPI).SlashFilter = slashfilter.New(ds.NewMapDatastore())

	fmt.Println("After mine bad block!")
	tu.printHeads()
	a2 := tu.mineOnBlock(base, 0, nil, true, false, nil)

	tu.waitUntilSync(0, client)

	head, err := tu.nds[0].ChainHead(tu.ctx)
	require.NoError(t, err)

	if !head.Equals(a2.TipSet()) {
		t.Fatalf("expected head to be %s, but got %s", a2.Cids(), head.Cids())
	}
}

type badWpp struct{}

func (wpp badWpp) GenerateCandidates(context.Context, abi.PoStRandomness, uint64) ([]uint64, error) {
	return []uint64{1}, nil
}

func (wpp badWpp) ComputeProof(context.Context, []proof.SectorInfo, abi.PoStRandomness) ([]proof.PoStProof, error) {
	return []proof.PoStProof{
		{
			PoStProof:  abi.RegisteredPoStProof_StackedDrgWinning2KiBV1,
			ProofBytes: []byte("evil"),
		},
	}, nil
}

func TestSyncBadWinningPoSt(t *testing.T) {
	H := 15
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(client, 0)
	tu.waitUntilSync(0, client)

	base := tu.g.CurTipset

	// both miners now produce invalid winning posts
	tu.g.SetWinningPoStProver(tu.g.Miners[0], &badWpp{})
	tu.g.SetWinningPoStProver(tu.g.Miners[1], &badWpp{})

	// now ensure that new blocks are not accepted
	tu.mineOnBlock(base, client, nil, false, true, nil)
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
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false, nil)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false, nil)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false, nil)

	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)

	fmt.Println("A: ", a.Cids(), a.TipSet().Height())
	fmt.Println("B: ", b.Cids(), b.TipSet().Height())

	// Now for the fun part!!

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilSyncTarget(p1, b.TipSet())
	tu.waitUntilSyncTarget(p2, b.TipSet())

	phead()
}

// This test crafts a tipset with 2 blocks, A and B.
// A and B both include _different_ messages from sender X with nonce N (where N is the correct nonce for X).
// We can confirm that the state can be correctly computed, and that `MessagesForTipset` behaves as expected.
func TestDuplicateNonce(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	base := tu.g.CurTipset

	// Produce a message from the banker to the rcvr
	makeMsg := func(rcvr address.Address) *types.SignedMessage {

		ba, err := tu.nds[0].StateGetActor(context.TODO(), tu.g.Banker(), base.TipSet().Key())
		require.NoError(t, err)
		msg := types.Message{
			To:   rcvr,
			From: tu.g.Banker(),

			Nonce: ba.Nonce,

			Value: types.NewInt(1),

			Method: 0,

			GasLimit:   100_000_000,
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
		}

		sig, err := tu.g.Wallet().WalletSign(context.TODO(), tu.g.Banker(), msg.Cid().Bytes(), api.MsgMeta{})
		require.NoError(t, err)

		return &types.SignedMessage{
			Message:   msg,
			Signature: *sig,
		}
	}

	msgs := make([][]*types.SignedMessage, 2)
	// Each miner includes a message from the banker with the same nonce, but to different addresses
	for k := range msgs {
		msgs[k] = []*types.SignedMessage{makeMsg(tu.g.Miners[k])}
	}

	ts1 := tu.mineOnBlock(base, 0, []int{0, 1}, true, false, msgs)

	tu.waitUntilSyncTarget(0, ts1.TipSet())

	// mine another tipset

	ts2 := tu.mineOnBlock(ts1, 0, []int{0, 1}, true, false, make([][]*types.SignedMessage, 2))
	tu.waitUntilSyncTarget(0, ts2.TipSet())

	var includedMsg cid.Cid
	var skippedMsg cid.Cid
	r0, err0 := tu.nds[0].StateGetReceipt(context.TODO(), msgs[0][0].Cid(), ts2.TipSet().Key())
	r1, err1 := tu.nds[0].StateGetReceipt(context.TODO(), msgs[1][0].Cid(), ts2.TipSet().Key())

	if err0 == nil {
		require.Error(t, err1, "at least one of the StateGetReceipt calls should fail")
		require.True(t, r0.ExitCode.IsSuccess())
		includedMsg = msgs[0][0].Message.Cid()
		skippedMsg = msgs[1][0].Message.Cid()
	} else {
		require.NoError(t, err1, "both the StateGetReceipt calls should not fail")
		require.True(t, r1.ExitCode.IsSuccess())
		includedMsg = msgs[1][0].Message.Cid()
		skippedMsg = msgs[0][0].Message.Cid()
	}

	_, rslts, err := tu.g.StateManager().ExecutionTrace(context.TODO(), ts1.TipSet())
	require.NoError(t, err)
	found := false
	for _, v := range rslts {
		if v.Msg.Cid() == skippedMsg {
			t.Fatal("skipped message should not be in exec trace")
		}

		if v.Msg.Cid() == includedMsg {
			found = true
		}
	}

	if !found {
		t.Fatal("included message should be in exec trace")
	}

	mft, err := tu.g.ChainStore().MessagesForTipset(ts1.TipSet())
	require.NoError(t, err)
	require.True(t, len(mft) == 1, "only expecting one message for this tipset")
	require.Equal(t, includedMsg, mft[0].VMMessage().Cid(), "messages for tipset didn't contain expected message")
}

// This test asserts that a block that includes a message with bad nonce can't be synced. A nonce is "bad" if it can't
// be applied on the parent state.
func TestBadNonce(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	base := tu.g.CurTipset

	// Produce a message from the banker with a bad nonce
	makeBadMsg := func() *types.SignedMessage {

		ba, err := tu.nds[0].StateGetActor(context.TODO(), tu.g.Banker(), base.TipSet().Key())
		require.NoError(t, err)
		msg := types.Message{
			To:   tu.g.Banker(),
			From: tu.g.Banker(),

			Nonce: ba.Nonce + 5,

			Value: types.NewInt(1),

			Method: 0,

			GasLimit:   100_000_000,
			GasFeeCap:  types.NewInt(0),
			GasPremium: types.NewInt(0),
		}

		sig, err := tu.g.Wallet().WalletSign(context.TODO(), tu.g.Banker(), msg.Cid().Bytes(), api.MsgMeta{})
		require.NoError(t, err)

		return &types.SignedMessage{
			Message:   msg,
			Signature: *sig,
		}
	}

	msgs := make([][]*types.SignedMessage, 1)
	msgs[0] = []*types.SignedMessage{makeBadMsg()}

	tu.mineOnBlock(base, 0, []int{0}, true, true, msgs)
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
	}, false)
	if err == nil {
		t.Fatal("should error on empty block")
	}

	h := mocktypes.MkBlock(nil, 123, 432)

	h.ElectionProof = nil

	err = s.ValidateBlock(context.TODO(), &types.FullBlock{Header: h}, false)
	if err == nil {
		t.Fatal("should error on block with nil election proof")
	}
}

func TestSyncCheckpointHead(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	p1 := tu.addClientNode()
	p2 := tu.addClientNode()

	fmt.Println("GENESIS: ", tu.g.Genesis().Cid())
	tu.loadChainToNode(p1)
	tu.loadChainToNode(p2)

	base := tu.g.CurTipset
	fmt.Println("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	// The two nodes fork at this point into 'a' and 'b'
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false, nil)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false, nil)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false, nil)

	tu.waitUntilSyncTarget(p1, a.TipSet())
	tu.checkpointTs(p1, a.TipSet().Key())

	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)

	fmt.Println("A: ", a.Cids(), a.TipSet().Height())
	fmt.Println("B: ", b.Cids(), b.TipSet().Height())

	// Now for the fun part!! p1 should mark p2's head as BAD.

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilNodeHasTs(p1, b.TipSet().Key())
	p1Head := tu.getHead(p1)
	require.Equal(tu.t, p1Head, a.TipSet())
	tu.assertBad(p1, b.TipSet())
}

func TestSyncCheckpointEarlierThanHead(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	p1 := tu.addClientNode()
	p2 := tu.addClientNode()

	fmt.Println("GENESIS: ", tu.g.Genesis().Cid())
	tu.loadChainToNode(p1)
	tu.loadChainToNode(p2)

	base := tu.g.CurTipset
	fmt.Println("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	// The two nodes fork at this point into 'a' and 'b'
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false, nil)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false, nil)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false, nil)

	tu.waitUntilSyncTarget(p1, a.TipSet())
	tu.checkpointTs(p1, a1.TipSet().Key())

	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil)

	fmt.Println("A: ", a.Cids(), a.TipSet().Height())
	fmt.Println("B: ", b.Cids(), b.TipSet().Height())

	// Now for the fun part!! p1 should mark p2's head as BAD.

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilNodeHasTs(p1, b.TipSet().Key())
	p1Head := tu.getHead(p1)
	require.Equal(tu.t, p1Head, a.TipSet())
	tu.assertBad(p1, b.TipSet())
}
