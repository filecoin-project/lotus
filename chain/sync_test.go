package chain_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/gen/slashfilter"
	"github.com/filecoin-project/lotus/chain/stmgr"
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
	us  stmgr.UpgradeSchedule
}

func prepSyncTest(t testing.TB, h int) *syncTestUtil {
	_ = logging.SetLogLevel("*", "INFO")
	_ = logging.SetLogLevel("fil-consensus", "ERROR")

	g, err := gen.NewGenerator()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tu := &syncTestUtil{
		t:      t,
		ctx:    ctx,
		cancel: cancel,

		mn: mocknet.New(),
		g:  g,
		us: filcns.DefaultUpgradeSchedule(),
	}

	tu.addSourceNode(h)

	//tu.checkHeight("source", source, h)

	// separate logs
	fmt.Println("///////////////////////////////////////////////////")

	return tu
}

func prepSyncTestWithV5Height(t testing.TB, h int, v5height abi.ChainEpoch) *syncTestUtil {
	_ = logging.SetLogLevel("*", "INFO")

	sched := stmgr.UpgradeSchedule{{
		// prepare for upgrade.
		Network:   network.Version9,
		Height:    1,
		Migration: filcns.UpgradeActorsV2,
	}, {
		Network:   network.Version10,
		Height:    2,
		Migration: filcns.UpgradeActorsV3,
	}, {
		Network:   network.Version12,
		Height:    3,
		Migration: filcns.UpgradeActorsV4,
	}, {
		Network:   network.Version13,
		Height:    v5height,
		Migration: filcns.UpgradeActorsV5,
	}, {
		Network:   network.Version14,
		Height:    v5height + 10,
		Migration: filcns.UpgradeActorsV6,
	}, {
		Network:   network.Version15,
		Height:    v5height + 15,
		Migration: filcns.UpgradeActorsV7,
	}, {
		Network:   network.Version16,
		Height:    v5height + 20,
		Migration: filcns.UpgradeActorsV8,
	}, {
		Network:   network.Version17,
		Height:    v5height + 25,
		Migration: filcns.UpgradeActorsV9,
	}}

	g, err := gen.NewGeneratorWithUpgradeSchedule(sched)

	if err != nil {
		t.Fatalf("%+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tu := &syncTestUtil{
		t:      t,
		ctx:    ctx,
		cancel: cancel,

		mn: mocknet.New(),
		g:  g,
		us: sched,
	}

	tu.addSourceNode(h)
	//tu.checkHeight("source", source, h)

	// separate logs
	fmt.Println("///////////////////////////////////////////////////")
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
				tu.t.Helper()
				tu.t.Fatal("took too long waiting for block to be accepted")
			}
		}
	}
}

func (tu *syncTestUtil) pushTsExpectErr(to int, fts *store.FullTipSet, experr bool) {
	ctx := context.TODO()
	for _, fb := range fts.Blocks {
		var b types.BlockMsg

		b.Header = fb.Header
		for _, msg := range fb.SecpkMessages {
			c, err := tu.nds[to].(*impl.FullNodeAPI).ChainAPI.Chain.PutMessage(ctx, msg)
			require.NoError(tu.t, err)

			b.SecpkMessages = append(b.SecpkMessages, c)
		}

		for _, msg := range fb.BlsMessages {
			c, err := tu.nds[to].(*impl.FullNodeAPI).ChainAPI.Chain.PutMessage(ctx, msg)
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

func (tu *syncTestUtil) mineOnBlock(blk *store.FullTipSet, to int, miners []int, wait, fail bool, msgs [][]*types.SignedMessage, nulls abi.ChainEpoch, push bool) *store.FullTipSet {
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
		nts, err = tu.g.NextTipSetFromMinersWithMessagesAndNulls(blk.TipSet(), maddrs, msgs, nulls)
		require.NoError(tu.t, err)
	} else {
		mt, err := tu.g.NextTipSetFromMiners(blk.TipSet(), maddrs, nulls)
		require.NoError(tu.t, err)
		nts = mt.TipSet
	}

	if push {
		if fail {
			tu.pushTsExpectErr(to, nts, true)
		} else {
			tu.pushFtsAndWait(to, nts, wait)
		}
	}

	return nts
}

func (tu *syncTestUtil) mineNewBlock(src int, miners []int) {
	mts := tu.mineOnBlock(tu.g.CurTipset, src, miners, true, false, nil, 0, true)
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
		node.Base(),
		node.Repo(sourceRepo),
		node.MockHost(tu.mn),
		node.Test(),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
		node.Override(new(stmgr.UpgradeSchedule), tu.us),
	)
	require.NoError(tu.t, err)
	tu.t.Cleanup(func() { _ = stop(context.Background()) })

	lastTs := blocks[len(blocks)-1]
	cs := out.(*impl.FullNodeAPI).ChainAPI.Chain
	for _, lastB := range lastTs.Blocks {
		require.NoError(tu.t, cs.AddToTipSetTracker(context.Background(), lastB.Header))
	}
	err = cs.RefreshHeaviestTipSet(tu.ctx, lastTs.TipSet().Height())
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

	r := repo.NewMemory(nil)
	stop, err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Base(),
		node.Repo(r),
		node.MockHost(tu.mn),
		node.Test(),

		node.Override(new(modules.Genesis), modules.LoadGenesis(tu.genesis)),
		node.Override(new(stmgr.UpgradeSchedule), tu.us),
	)
	require.NoError(tu.t, err)
	tu.t.Cleanup(func() { _ = stop(context.Background()) })

	tu.nds = append(tu.nds, out)
	return len(tu.nds) - 1
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

func (tu *syncTestUtil) nodeHasTs(node int, tsk types.TipSetKey) bool {
	_, err := tu.nds[node].ChainGetTipSet(context.TODO(), tsk)
	return err == nil
}

func (tu *syncTestUtil) waitUntilNodeHasTs(node int, tsk types.TipSetKey) {
	for !tu.nodeHasTs(node, tsk) {
		// Time to allow for syncing and validation
		time.Sleep(10 * time.Millisecond)
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

	timeout := time.After(5 * time.Second)

	for {
		select {
		case n := <-hc:
			for _, c := range n {
				if c.Val.Equals(target) {
					return
				}
			}
		case <-timeout:
			tu.t.Fatal("waitUntilSyncTarget timeout")
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
		return pts.MinTimestamp() + (buildconstants.BlockDelaySecs / 2)
	}

	fmt.Println("BASE: ", base.Cids())
	tu.printHeads()

	a1 := tu.mineOnBlock(base, 0, nil, false, true, nil, 0, true)

	tu.g.Timestamper = nil
	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))

	tu.nds[0].(*impl.FullNodeAPI).SlashFilter = slashfilter.New(ds.NewMapDatastore())

	fmt.Println("After mine bad block!")
	tu.printHeads()
	a2 := tu.mineOnBlock(base, 0, nil, true, false, nil, 0, true)

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

func (wpp badWpp) ComputeProof(context.Context, []prooftypes.ExtendedSectorInfo, abi.PoStRandomness, abi.ChainEpoch, network.Version) ([]prooftypes.PoStProof, error) {
	return []prooftypes.PoStProof{
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
	tu.mineOnBlock(base, client, nil, false, true, nil, 0, true)
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

	printHead := func() {
		h1, err := tu.nds[1].ChainHead(tu.ctx)
		require.NoError(tu.t, err)

		h2, err := tu.nds[2].ChainHead(tu.ctx)
		require.NoError(tu.t, err)

		w1, err := tu.nds[1].(*impl.FullNodeAPI).ChainAPI.Chain.Weight(tu.ctx, h1)
		require.NoError(tu.t, err)
		w2, err := tu.nds[2].(*impl.FullNodeAPI).ChainAPI.Chain.Weight(tu.ctx, h2)
		require.NoError(tu.t, err)

		fmt.Println("Node 1: ", h1.Cids(), h1.Parents(), h1.Height(), w1)
		fmt.Println("Node 2: ", h2.Cids(), h2.Parents(), h2.Height(), w2)
		//time.Sleep(time.Second * 2)
		fmt.Println()
		fmt.Println()
		fmt.Println()
		fmt.Println()
	}

	printHead()

	base := tu.g.CurTipset
	fmt.Println("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	// The two nodes fork at this point into 'a' and 'b'
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false, nil, 0, true)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false, nil, 0, true)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false, nil, 0, true)

	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)

	fmt.Println("A: ", a.Cids(), a.TipSet().Height())
	fmt.Println("B: ", b.Cids(), b.TipSet().Height())

	printHead()

	// Now for the fun part!!

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilSyncTarget(p1, b.TipSet())
	tu.waitUntilSyncTarget(p2, b.TipSet())

	printHead()
}

// This test crafts a tipset with 2 blocks, A and B.
// A and B both include _different_ messages from sender X with nonce N (where N is the correct nonce for X).
// We can confirm that the state can be correctly computed, and that `MessagesForTipset` behaves as expected.
func TestDuplicateNonce(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	base := tu.g.CurTipset

	// Get the banker from computed tipset state, not the parent.
	st, _, err := tu.g.StateManager().TipSetState(context.TODO(), base.TipSet())
	require.NoError(t, err)
	ba, err := tu.g.StateManager().LoadActorRaw(context.TODO(), tu.g.Banker(), st)
	require.NoError(t, err)

	// Produce a message from the banker to the rcvr
	makeMsg := func(rcvr address.Address) *types.SignedMessage {
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

	ts1 := tu.mineOnBlock(base, 0, []int{0, 1}, true, false, msgs, 0, true)

	tu.waitUntilSyncTarget(0, ts1.TipSet())

	// mine another tipset

	ts2 := tu.mineOnBlock(ts1, 0, []int{0, 1}, true, false, make([][]*types.SignedMessage, 2), 0, true)
	tu.waitUntilSyncTarget(0, ts2.TipSet())

	var includedMsg cid.Cid
	var skippedMsg cid.Cid
	r0, err0 := tu.nds[0].StateSearchMsg(context.TODO(), ts2.TipSet().Key(), msgs[0][0].Cid(), api.LookbackNoLimit, true)
	r1, err1 := tu.nds[0].StateSearchMsg(context.TODO(), ts2.TipSet().Key(), msgs[1][0].Cid(), api.LookbackNoLimit, true)

	if err0 == nil {
		require.Error(t, err1, "at least one of the StateGetReceipt calls should fail")
		require.True(t, r0.Receipt.ExitCode.IsSuccess())
		includedMsg = msgs[0][0].Message.Cid()
		skippedMsg = msgs[1][0].Message.Cid()
	} else {
		require.NoError(t, err1, "both the StateGetReceipt calls should not fail")
		require.True(t, r1.Receipt.ExitCode.IsSuccess())
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

	mft, err := tu.g.ChainStore().MessagesForTipset(context.TODO(), ts1.TipSet())
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

	// Get the banker from computed tipset state, not the parent.
	st, _, err := tu.g.StateManager().TipSetState(context.TODO(), base.TipSet())
	require.NoError(t, err)
	ba, err := tu.g.StateManager().LoadActorRaw(context.TODO(), tu.g.Banker(), st)
	require.NoError(t, err)

	// Produce a message from the banker with a bad nonce
	makeBadMsg := func() *types.SignedMessage {
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

	tu.mineOnBlock(base, 0, []int{0}, true, true, msgs, 0, true)
}

// This test introduces a block that has 2 messages, with the same sender, and same nonce.
// One of the messages uses the sender's robust address, the other uses the ID address.
// Such a block is invalid and should not sync.
func TestMismatchedNoncesRobustID(t *testing.T) {
	v5h := abi.ChainEpoch(4)
	tu := prepSyncTestWithV5Height(t, int(v5h+5), v5h)

	base := tu.g.CurTipset

	// Get the banker from computed tipset state, not the parent.
	st, _, err := tu.g.StateManager().TipSetState(context.TODO(), base.TipSet())
	require.NoError(t, err)
	ba, err := tu.g.StateManager().LoadActorRaw(context.TODO(), tu.g.Banker(), st)
	require.NoError(t, err)

	// Produce a message from the banker
	makeMsg := func(id bool) *types.SignedMessage {
		sender := tu.g.Banker()
		if id {
			s, err := tu.nds[0].StateLookupID(context.TODO(), sender, base.TipSet().Key())
			require.NoError(t, err)
			sender = s
		}

		msg := types.Message{
			To:   tu.g.Banker(),
			From: sender,

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

	msgs := make([][]*types.SignedMessage, 1)
	msgs[0] = []*types.SignedMessage{makeMsg(false), makeMsg(true)}

	tu.mineOnBlock(base, 0, []int{0}, true, true, msgs, 0, true)
}

// This test introduces a block that has 2 messages, with the same sender, and nonces N and N+1 (so both can be included in a block)
// One of the messages uses the sender's robust address, the other uses the ID address.
// Such a block is valid and should sync.
func TestMatchedNoncesRobustID(t *testing.T) {
	v5h := abi.ChainEpoch(4)
	tu := prepSyncTestWithV5Height(t, int(v5h+5), v5h)

	base := tu.g.CurTipset

	// Get the banker from computed tipset state, not the parent.
	st, _, err := tu.g.StateManager().TipSetState(context.TODO(), base.TipSet())
	require.NoError(t, err)
	ba, err := tu.g.StateManager().LoadActorRaw(context.TODO(), tu.g.Banker(), st)
	require.NoError(t, err)

	// Produce a message from the banker with specified nonce
	makeMsg := func(n uint64, id bool) *types.SignedMessage {
		sender := tu.g.Banker()
		if id {
			s, err := tu.nds[0].StateLookupID(context.TODO(), sender, base.TipSet().Key())
			require.NoError(t, err)
			sender = s
		}

		msg := types.Message{
			To:   tu.g.Banker(),
			From: sender,

			Nonce: n,

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
	msgs[0] = []*types.SignedMessage{makeMsg(ba.Nonce, false), makeMsg(ba.Nonce+1, true)}

	tu.mineOnBlock(base, 0, []int{0}, true, false, msgs, 0, true)
}

func BenchmarkSyncBasic(b *testing.B) {
	for b.Loop() {
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
	t.Log("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	// The two nodes fork at this point into 'a' and 'b'
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false, nil, 0, true)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false, nil, 0, true)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false, nil, 0, true)

	tu.waitUntilSyncTarget(p1, a.TipSet())
	tu.checkpointTs(p1, a.TipSet().Key())

	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)

	t.Log("A: ", a.Cids(), a.TipSet().Height())
	t.Log("B: ", b.Cids(), b.TipSet().Height())

	// Now for the fun part!! p1 should mark p2's head as BAD.

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilNodeHasTs(p1, b.TipSet().Key())

	p1Head := tu.getHead(p1)
	require.True(tu.t, p1Head.Equals(a.TipSet()))
	tu.assertBad(p1, b.TipSet())

	// Should be able to switch forks.
	tu.checkpointTs(p1, b.TipSet().Key())
	p1Head = tu.getHead(p1)
	require.True(tu.t, p1Head.Equals(b.TipSet()))
}

func TestSyncCheckpointPartial(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	p1 := tu.addClientNode()
	p2 := tu.addClientNode()

	fmt.Println("GENESIS: ", tu.g.Genesis().Cid())
	tu.loadChainToNode(p1)
	tu.loadChainToNode(p2)

	base := tu.g.CurTipset
	fmt.Println("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	last := base
	a := base
	for {
		a = tu.mineOnBlock(last, p1, []int{0, 1}, true, false, nil, 0, true)
		if len(a.Blocks) == 2 {
			// enfoce tipset of two blocks
			break
		}
		tu.pushTsExpectErr(p2, a, false) // push these to p2 as well
		last = a
	}
	var aPartial *store.FullTipSet
	var aPartial2 *store.FullTipSet
	for _, b := range a.Blocks {
		if b.Header.Miner == tu.g.Miners[1] {
			// need to have miner two block in the partial tipset
			// as otherwise it will be a parent grinding fault
			aPartial = store.NewFullTipSet([]*types.FullBlock{b})
		} else {
			aPartial2 = store.NewFullTipSet([]*types.FullBlock{b})
		}
	}
	tu.waitUntilSyncTarget(p1, a.TipSet())

	tu.pushFtsAndWait(p2, a, true)
	tu.checkpointTs(p2, aPartial.TipSet().Key())
	t.Logf("p1 head: %v, p2 head: %v, a: %v", tu.getHead(p1), tu.getHead(p2), a.TipSet())
	tu.pushTsExpectErr(p2, aPartial2, true)

	b := tu.mineOnBlock(a, p1, []int{0}, true, false, nil, 0, true)
	tu.pushTsExpectErr(p2, b, true)

	require.NoError(t, tu.g.ResyncBankerNonce(b.TipSet())) // don't ask me why it has to be TS b
	c := tu.mineOnBlock(aPartial, p2, []int{1}, true, false, nil, 0, true)

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)

	tu.pushFtsAndWait(p2, c, true)
	tu.waitUntilNodeHasTs(p1, c.TipSet().Key())
	tu.checkpointTs(p1, c.TipSet().Key())

}

func TestSyncCheckpointSubmitOneOfTheBlocks(t *testing.T) {
	H := 10
	tu := prepSyncTest(t, H)

	p1 := tu.addClientNode()
	p2 := tu.addClientNode()

	fmt.Println("GENESIS: ", tu.g.Genesis().Cid())
	tu.loadChainToNode(p1)
	tu.loadChainToNode(p2)

	base := tu.g.CurTipset
	fmt.Println("Mining base: ", base.TipSet().Cids(), base.TipSet().Height())

	last := base
	a := base
	for {
		a = tu.mineOnBlock(last, p1, []int{0, 1}, true, false, nil, 0, true)
		if len(a.Blocks) == 2 {
			// enfoce tipset of two blocks
			break
		}
		last = a
	}
	aPartial := store.NewFullTipSet([]*types.FullBlock{a.Blocks[0]})
	tu.waitUntilSyncTarget(p1, a.TipSet())

	tu.checkpointTs(p1, a.TipSet().Key())
	t.Logf("p1 head: %v, p2 head: %v, a: %v", tu.getHead(p1), tu.getHead(p2), a.TipSet())
	tu.pushTsExpectErr(p1, aPartial, false)

	tu.mineOnBlock(a, p1, []int{0, 1}, true, false, nil, 0, true)
	tu.pushTsExpectErr(p1, aPartial, false) // check that pushing older partial tispet doesn't error

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
	a1 := tu.mineOnBlock(base, p1, []int{0}, true, false, nil, 0, true)
	a := tu.mineOnBlock(a1, p1, []int{0}, true, false, nil, 0, true)
	a = tu.mineOnBlock(a, p1, []int{0}, true, false, nil, 0, true)

	tu.waitUntilSyncTarget(p1, a.TipSet())
	tu.checkpointTs(p1, a1.TipSet().Key())

	require.NoError(t, tu.g.ResyncBankerNonce(a1.TipSet()))
	// chain B will now be heaviest
	b := tu.mineOnBlock(base, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)
	b = tu.mineOnBlock(b, p2, []int{1}, true, false, nil, 0, true)

	fmt.Println("A: ", a.Cids(), a.TipSet().Height())
	fmt.Println("B: ", b.Cids(), b.TipSet().Height())

	// Now for the fun part!! p1 should mark p2's head as BAD.

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(p1, p2)
	tu.waitUntilNodeHasTs(p1, b.TipSet().Key())
	p1Head := tu.getHead(p1)
	require.True(tu.t, p1Head.Equals(a.TipSet()))
	tu.assertBad(p1, b.TipSet())

	// Should be able to switch forks.
	tu.checkpointTs(p1, b.TipSet().Key())
	p1Head = tu.getHead(p1)
	require.True(tu.t, p1Head.Equals(b.TipSet()))
}

func TestInvalidHeight(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(client, 0)
	tu.waitUntilSync(0, client)

	base := tu.g.CurTipset

	for i := 0; i < 5; i++ {
		base = tu.mineOnBlock(base, 0, nil, false, false, nil, 0, false)
	}

	tu.mineOnBlock(base, 0, nil, false, true, nil, -1, true)
}

// TestIncomingBlocks mines new blocks and checks if the incoming channel streams new block headers properly
func TestIncomingBlocks(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	require.NoError(t, tu.mn.LinkAll())

	clientNode := tu.nds[client]
	incoming, err := clientNode.SyncIncomingBlocks(tu.ctx)
	require.NoError(tu.t, err)

	tu.connect(client, 0)
	tu.waitUntilSync(0, client)
	tu.compareSourceState(client)

	timeout := time.After(10 * time.Second)

	for i := 0; i < 5; i++ {
		tu.mineNewBlock(0, nil)
		tu.waitUntilSync(0, client)
		tu.compareSourceState(client)

		// just in case, so we don't get deadlocked
		select {
		case <-incoming:
		case <-timeout:
			tu.t.Fatal("TestIncomingBlocks timeout")
		}
	}
}

// TestSyncManualBadTS tests manually marking and unmarking blocks in the bad TS cache
func TestSyncManualBadTS(t *testing.T) {
	// Test setup:
	// - source node is fully synced,
	// - client node is unsynced
	// - client manually marked source's head and it's parent as bad
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	require.NoError(t, tu.mn.LinkAll())

	sourceHead, err := tu.nds[source].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	clientHead, err := tu.nds[client].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	require.True(tu.t, !sourceHead.Equals(clientHead), "source and client should be out of sync in test setup")

	err = tu.nds[client].SyncMarkBad(tu.ctx, sourceHead.Cids()[0])
	require.NoError(tu.t, err)

	sourceHeadParent := sourceHead.Parents().Cids()[0]
	err = tu.nds[client].SyncMarkBad(tu.ctx, sourceHeadParent)
	require.NoError(tu.t, err)

	reason, err := tu.nds[client].SyncCheckBad(tu.ctx, sourceHead.Cids()[0])
	require.NoError(tu.t, err)
	require.NotEqual(tu.t, "", reason, "block is not bad after manually marking")

	reason, err = tu.nds[client].SyncCheckBad(tu.ctx, sourceHeadParent)
	require.NoError(tu.t, err)
	require.NotEqual(tu.t, "", reason, "block is not bad after manually marking")

	// Assertion 1:
	// - client shouldn't be synced after timeout, because the source TS is marked bad.
	// - bad block is the first block that should be synced, 1sec should be enough
	tu.connect(1, 0)
	timeout := time.After(1 * time.Second)
	<-timeout

	clientHead, err = tu.nds[client].ChainHead(tu.ctx)
	require.NoError(tu.t, err)
	require.True(tu.t, !sourceHead.Equals(clientHead), "source and client should be out of sync if source head is bad")

	// Assertion 2:
	// - after unmarking blocks as bad and reconnecting, source & client should be in sync
	err = tu.nds[client].SyncUnmarkBad(tu.ctx, sourceHead.Cids()[0])
	require.NoError(tu.t, err)

	reason, err = tu.nds[client].SyncCheckBad(tu.ctx, sourceHead.Cids()[0])
	require.NoError(tu.t, err)
	require.Equal(tu.t, "", reason, "block is still bad after manually unmarking")

	err = tu.nds[client].SyncUnmarkAllBad(tu.ctx)
	require.NoError(tu.t, err)

	reason, err = tu.nds[client].SyncCheckBad(tu.ctx, sourceHeadParent)
	require.NoError(tu.t, err)
	require.Equal(tu.t, "", reason, "block is still bad after manually unmarking")

	tu.disconnect(1, 0)
	tu.connect(1, 0)

	tu.waitUntilSync(0, client)
	tu.compareSourceState(client)
}

// TestSyncState tests fetching the sync worker state before, during & after the sync
func TestSyncState(t *testing.T) {
	H := 50
	tu := prepSyncTest(t, H)

	client := tu.addClientNode()
	require.NoError(t, tu.mn.LinkAll())
	clientNode := tu.nds[client]
	sourceHead, err := tu.nds[source].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	// sync state should be empty before the sync
	state, err := clientNode.SyncState(tu.ctx)
	require.NoError(tu.t, err)
	require.Equal(tu.t, len(state.ActiveSyncs), 0)

	tu.connect(client, 0)

	// wait until sync starts, or at most `timeout` seconds
	timeout := time.After(5 * time.Second)
	activeSyncs := []api.ActiveSync{}

	for len(activeSyncs) == 0 {
		state, err = clientNode.SyncState(tu.ctx)
		require.NoError(tu.t, err)
		activeSyncs = state.ActiveSyncs

		sleep := time.After(100 * time.Millisecond)
		select {
		case <-sleep:
		case <-timeout:
			tu.t.Fatal("TestSyncState timeout")
		}
	}

	// check state during sync
	require.Equal(tu.t, len(activeSyncs), 1)
	require.True(tu.t, activeSyncs[0].Target.Equals(sourceHead))

	tu.waitUntilSync(0, client)
	tu.compareSourceState(client)

	// check state after sync
	state, err = clientNode.SyncState(tu.ctx)
	require.NoError(tu.t, err)
	require.Equal(tu.t, len(state.ActiveSyncs), 1)
	require.Equal(tu.t, state.ActiveSyncs[0].Stage, api.StageSyncComplete)
}
