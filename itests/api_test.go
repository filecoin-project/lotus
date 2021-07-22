package itests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

func TestAPI(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		runAPITest(t)
	})
	t.Run("rpc", func(t *testing.T) {
		runAPITest(t, kit.ThroughRPC())
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
	t.Run("testMiningReal", ts.testMiningReal)
	t.Run("testSearchMsg", ts.testSearchMsg)
	t.Run("testNonGenesisMiner", ts.testNonGenesisMiner)
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
	require.Equal(t, versions[0], build.BuildVersion)
}

func (ts *apiSuite) testID(t *testing.T) {
	ctx := context.Background()

	full, _, _ := kit.EnsembleMinimal(t, ts.opts...)

	id, err := full.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	require.Regexp(t, "^12", id.Pretty())
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
	require.Lenf(t, peers, 2, "node one doesn't have 2 peers")

	peers, err = two.NetPeers(ctx)
	require.NoError(t, err)
	require.Lenf(t, peers, 2, "node two doesn't have 2 peers")
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
		kit.ProofType(abi.RegisteredSealProof_StackedDrg2KiBV1), // we're using v0 actors with old proofs.
		kit.WithAllSubsystems(),
	).Start().InterconnectAll()

	ta, err := newMiner.ActorAddress(ctx)
	require.NoError(t, err)

	tid, err := address.IDFromAddress(ta)
	require.NoError(t, err)

	require.Equal(t, uint64(1001), tid)
}
