package itests

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestAPI(t *testing.T) {

	ts := apiSuite{opts: []interface{}{kit.RealProofs()}}
	t.Run("testMiningReal", ts.testMiningReal)
	ts.opts = append(ts.opts, kit.ThroughRPC())
	t.Run("testMiningReal", ts.testMiningReal)

	t.Run("direct", func(t *testing.T) {
		runAPITest(t, kit.MockProofs())
	})
	t.Run("rpc", func(t *testing.T) {
		runAPITest(t, kit.MockProofs(), kit.ThroughRPC())
	})
}

type apiSuite struct {
	opts []interface{}
}

// runAPITest is the entry point to API test suite
func runAPITest(t *testing.T, opts ...interface{}) {
	ts := apiSuite{opts: opts}

	t.Run("version", ts.testVersion)
	t.Run("id", ts.testID)
	t.Run("testConnectTwo", ts.testConnectTwo)
	t.Run("testMining", ts.testMining)
	t.Run("testSlowNotify", ts.testSlowNotify)
	t.Run("testSearchMsg", ts.testSearchMsg)
	t.Run("testOutOfGasError", ts.testOutOfGasError)
	t.Run("testLookupNotFoundError", ts.testLookupNotFoundError)
	t.Run("testNonGenesisMiner", ts.testNonGenesisMiner)
	t.Run("testEmptySlices", ts.testEmptySlices)
}

// testEmptySlices checks that methods returning a list encode an empty result as
// `[]` rather than `null`. encoding/json renders a nil slice as `null`, which is a
// different value to clients that iterate the result without a nil check.
//
// Add any new list-returning method here.
func (ts *apiSuite) testEmptySlices(t *testing.T) {
	ctx := context.Background()

	full, miner, _ := kit.EnsembleMinimal(t, ts.opts...)

	gen, err := full.ChainGetGenesis(ctx)
	require.NoError(t, err)
	gtsk := gen.Key()
	gcid := gen.Blocks()[0].Cid()

	requireEmptyArray := func(v interface{}) {
		t.Helper()
		b, err := json.Marshal(v)
		require.NoError(t, err)
		require.Equal(t, "[]", string(b), "empty result must encode as [] and not null")
	}

	msgs, err := full.ChainGetMessagesInTipset(ctx, gtsk)
	require.NoError(t, err)
	requireEmptyArray(msgs)

	parentMsgs, err := full.ChainGetParentMessages(ctx, gcid)
	require.NoError(t, err)
	requireEmptyArray(parentMsgs)

	receipts, err := full.ChainGetParentReceipts(ctx, gcid)
	require.NoError(t, err)
	requireEmptyArray(receipts)

	// The burnt funds actor never sends messages or emits events.
	listed, err := full.StateListMessages(ctx, &lapi.MessageMatch{From: builtin.BurntFundsActorAddr}, gtsk, gen.Height())
	require.NoError(t, err)
	requireEmptyArray(listed)

	from := gen.Height()
	to := gen.Height()
	events, err := full.GetActorEventsRaw(ctx, &types.ActorEventFilter{
		Addresses:  []address.Address{builtin.BurntFundsActorAddr},
		FromHeight: &from,
		ToHeight:   &to,
	})
	require.NoError(t, err)
	requireEmptyArray(events)

	// A fresh miner leaves most deadlines with no partitions, which is the case
	// that regressed; every deadline must still encode as an array.
	deadlines, err := full.StateMinerDeadlines(ctx, miner.ActorAddr, gtsk)
	require.NoError(t, err)
	for dlIdx := range deadlines {
		parts, err := full.StateMinerPartitions(ctx, miner.ActorAddr, uint64(dlIdx), gtsk)
		require.NoError(t, err)
		b, err := json.Marshal(parts)
		require.NoError(t, err)
		require.NotEqual(t, "null", string(b), "deadline %d must encode as an array", dlIdx)
	}
}

func (ts *apiSuite) testVersion(t *testing.T) {
	lapi.RunningNodeType = lapi.NodeFull
	t.Cleanup(func() {
		lapi.RunningNodeType = lapi.NodeUnknown
	})

	full, _, _ := kit.EnsembleMinimal(t, ts.opts...)

	v, err := full.Version(context.Background())
	require.NoError(t, err)

	versions := strings.Split(v.Version, "+")
	require.NotZero(t, len(versions), "empty version")
	require.Equal(t, versions[0], build.NodeBuildVersion)
}

func (ts *apiSuite) testID(t *testing.T) {
	ctx := context.Background()

	full, _, _ := kit.EnsembleMinimal(t, ts.opts...)

	id, err := full.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	require.Regexp(t, "^12", id.String())
}

func (ts *apiSuite) testConnectTwo(t *testing.T) {
	ctx := context.Background()

	one, two, _, ens := kit.EnsembleTwoOne(t, ts.opts...)

	p, err := one.NetPeers(ctx)
	require.NoError(t, err)
	require.Empty(t, p, "node one has peers")

	p, err = two.NetPeers(ctx)
	require.NoError(t, err)
	require.Empty(t, p, "node two has peers")

	ens.InterconnectAll()

	peers, err := one.NetPeers(ctx)
	require.NoError(t, err)

	countPeerIDs := func(peers []peer.AddrInfo) int {
		peerIDs := make(map[peer.ID]struct{})
		for _, p := range peers {
			peerIDs[p.ID] = struct{}{}
		}

		return len(peerIDs)
	}

	require.Equal(t, countPeerIDs(peers), 1, "node one doesn't have 1 peer")

	peers, err = two.NetPeers(ctx)
	require.NoError(t, err)
	require.Equal(t, countPeerIDs(peers), 1, "node one doesn't have 1 peer")
}

func (ts *apiSuite) testSearchMsg(t *testing.T) {
	ctx := context.Background()

	full, _, ens := kit.EnsembleMinimal(t, ts.opts...)

	senderAddr, err := full.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	msg := &types.Message{
		From:  senderAddr,
		To:    senderAddr,
		Value: big.Zero(),
	}

	ens.BeginMining(100 * time.Millisecond)

	sm, err := full.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err := full.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)

	require.Equal(t, exitcode.Ok, res.Receipt.ExitCode, "message not successful")

	searchRes, err := full.StateSearchMsg(ctx, types.EmptyTSK, sm.Cid(), lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.NotNil(t, searchRes)

	require.Equalf(t, res.TipSet, searchRes.TipSet, "search ts: %s, different from wait ts: %s", searchRes.TipSet, res.TipSet)
}

func (ts *apiSuite) testOutOfGasError(t *testing.T) {
	ctx := context.Background()

	full, _, _ := kit.EnsembleMinimal(t, ts.opts...)

	senderAddr, err := full.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// the gas estimator API executes the message with gasLimit = BlockGasLimit
	// Lowering it to 2 will cause it to run out of gas, testing the failure case we want
	originalLimit := buildconstants.BlockGasLimit
	buildconstants.BlockGasLimit = 2
	defer func() {
		buildconstants.BlockGasLimit = originalLimit
	}()

	t.Logf("BlockGasLimit changed: %d", buildconstants.BlockGasLimit)

	msg := &types.Message{
		From:  senderAddr,
		To:    senderAddr,
		Value: big.Zero(),
	}

	_, err = full.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
	require.Error(t, err, "should have failed")
	require.True(t, errors.Is(err, &lapi.ErrOutOfGas{}))
}

func (ts *apiSuite) testLookupNotFoundError(t *testing.T) {
	ctx := context.Background()

	full, _, _ := kit.EnsembleMinimal(t, ts.opts...)

	addr, err := full.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	_, err = full.StateLookupID(ctx, addr, types.EmptyTSK)
	require.Error(t, err)
	require.True(t, errors.Is(err, &lapi.ErrActorNotFound{}))
}

func (ts *apiSuite) testMining(t *testing.T) {
	ctx := context.Background()

	full, miner, _ := kit.EnsembleMinimal(t, ts.opts...)

	newHeads, err := full.ChainNotify(ctx)
	require.NoError(t, err)
	initHead := (<-newHeads)[0]
	baseHeight := initHead.Val.Height()

	h1, err := full.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(h1.Height()), int64(baseHeight))

	bm := kit.NewBlockMiner(t, miner)
	bm.MineUntilBlock(ctx, full, nil)
	require.NoError(t, err)

	<-newHeads

	h2, err := full.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(h2.Height()), int64(h1.Height()))

	bm.MineUntilBlock(ctx, full, nil)
	require.NoError(t, err)

	<-newHeads

	h3, err := full.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(h3.Height()), int64(h2.Height()))
}

func (ts *apiSuite) testMiningReal(t *testing.T) {
	build.InsecurePoStValidation = false
	defer func() {
		build.InsecurePoStValidation = true
	}()

	ts.testMining(t)
}

func (ts *apiSuite) testSlowNotify(t *testing.T) {
	_ = logging.SetLogLevel("rpc", "ERROR")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	full, miner, _ := kit.EnsembleMinimal(t, ts.opts...)

	// Subscribe a bunch of times to make sure we fill up any RPC buffers.
	var newHeadsChans []<-chan []*lapi.HeadChange
	for i := 0; i < 100; i++ {
		newHeads, err := full.ChainNotify(ctx)
		require.NoError(t, err)
		newHeadsChans = append(newHeadsChans, newHeads)
	}

	initHead := (<-newHeadsChans[0])[0]
	baseHeight := initHead.Val.Height()

	bm := kit.NewBlockMiner(t, miner)
	bm.MineBlocks(ctx, time.Microsecond)

	full.WaitTillChain(ctx, kit.HeightAtLeast(baseHeight+100))

	// Make sure they were all closed, draining any buffered events first.
	for _, ch := range newHeadsChans {
		var ok bool
		for ok {
			select {
			case _, ok = <-ch:
			default:
				t.Fatal("expected new heads channel to be closed")
			}
		}
	}

	// Make sure we can resubscribe and everything still works.
	newHeads, err := full.ChainNotify(ctx)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		_, ok := <-newHeads
		require.True(t, ok, "notify channel closed")
	}
}

func (ts *apiSuite) testNonGenesisMiner(t *testing.T) {
	ctx := context.Background()

	full, genesisMiner, ens := kit.EnsembleMinimal(t, append(ts.opts, kit.MockProofs())...)

	ens.InterconnectAll().BeginMining(4 * time.Millisecond)

	time.Sleep(1 * time.Second)

	gaa, err := genesisMiner.ActorAddress(ctx)
	require.NoError(t, err)

	_, err = full.StateMinerInfo(ctx, gaa, types.EmptyTSK)
	require.NoError(t, err)

	var newMiner kit.TestMiner
	ens.Miner(&newMiner, full,
		kit.OwnerAddr(full.DefaultKey),
		kit.SectorSize(2<<10),
		kit.WithAllSubsystems(),
	).Start().InterconnectAll()

	ta, err := newMiner.ActorAddress(ctx)
	require.NoError(t, err)

	tid, err := address.IDFromAddress(ta)
	require.NoError(t, err)

	require.Equal(t, uint64(1002), tid) // ETH0 is 1001
}

func TestAPIMergeProxy(t *testing.T) {
	ctx := context.Background()

	// The default is too high for many nodes.
	initialBalance := types.MustParseFIL("100000FIL")

	nopts := []kit.NodeOpt{
		kit.ThroughRPC(),
		kit.WithAllSubsystems(),
		kit.OwnerBalance(big.Int(initialBalance)),
	}
	ens := kit.NewEnsemble(t, kit.MockProofs())
	nodes := make([]*kit.TestFullNode, 10)
	for i := range nodes {
		var nd kit.TestFullNode
		ens.FullNode(&nd, nopts...)
		nodes[i] = &nd
	}
	var proxy lapi.FullNodeStruct
	cliutil.FullNodeProxy(nodes, &proxy)
	merged := *nodes[0]
	merged.FullNode = &proxy

	var miner kit.TestMiner
	ens.Miner(&miner, &merged, nopts...)

	ens.Start()

	t.Run("cancelled waits preserve the cause", func(t *testing.T) {
		cause := errors.New("caller stopped waiting")
		waitCtx, cancel := context.WithCancelCause(ctx)
		cancel(cause)

		_, err := merged.WaitMsgResult(waitCtx, cid.Undef, 0)
		require.ErrorIs(t, err, cause)
		_, err = merged.WaitTillChainOrError(waitCtx, func(*types.TipSet) bool { return false })
		require.ErrorIs(t, err, cause)
	})

	nd1ID, err := nodes[0].ID(ctx)
	require.NoError(t, err)
	nd2ID, err := nodes[1].ID(ctx)
	require.NoError(t, err)

	// Expect to start on node 1, and switch to node 2 on failure.
	mergedID, err := merged.ID(ctx)
	require.NoError(t, err)
	require.Equal(t, nd1ID, mergedID)
	require.NoError(t, nodes[0].Stop(ctx))
	mergedID, err = merged.ID(ctx)
	require.NoError(t, err)
	require.Equal(t, nd2ID, mergedID)

	// Now see if sticky sessions work
	stickyCtx := cliutil.OnSingleNode(ctx)
	for i, nd := range nodes[1:] {
		// kill off the previous node.
		require.NoError(t, nodes[i].Stop(ctx))

		got, err := merged.ID(stickyCtx)
		require.NoError(t, err)
		expected, err := nd.ID(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, got)
	}

	// This should fail because we'll run out of retries because it's _not_ sticky!
	_, err = merged.ID(ctx)
	require.Error(t, err)
}

func TestChainPutObjRestoresActorState(t *testing.T) {
	t.Setenv("LOTUS_ENABLE_CHAINSTORE_FALLBACK", "")
	ctx := context.Background()

	full, miner, _ := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.SplitstoreDisable())

	bm := kit.NewBlockMiner(t, miner)
	t.Cleanup(bm.Stop)
	bm.MineUntilBlock(ctx, full, nil)
	head, err := full.ChainHead(ctx)
	require.NoError(t, err)

	originalState, err := full.StateReadState(ctx, miner.ActorAddr, head.Key())
	require.NoError(t, err)
	require.NotNil(t, originalState.State)

	actor, err := full.StateGetActor(ctx, miner.ActorAddr, head.Key())
	require.NoError(t, err)
	data, err := full.ChainReadObj(ctx, actor.Head)
	require.NoError(t, err)
	block, err := blocks.NewBlockWithCid(data, actor.Head)
	require.NoError(t, err)

	require.NoError(t, full.ChainDeleteObj(ctx, actor.Head))
	has, err := full.ChainHasObj(ctx, actor.Head)
	require.NoError(t, err)
	require.False(t, has)
	_, err = full.StateReadState(ctx, miner.ActorAddr, head.Key())
	require.ErrorContains(t, err, "getting actor head")
	require.ErrorContains(t, err, block.Cid().String())

	require.Error(t, full.ChainPutObj(ctx, nil))

	require.NoError(t, full.ChainPutObj(ctx, block))
	restoredState, err := full.StateReadState(ctx, miner.ActorAddr, head.Key())
	require.NoError(t, err)
	require.Equal(t, originalState, restoredState)

	bm.MineUntilBlock(ctx, full, nil)
	newHead, err := full.ChainHead(ctx)
	require.NoError(t, err)
	require.Greater(t, int64(newHead.Height()), int64(head.Height()))
}
