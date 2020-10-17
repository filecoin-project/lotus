package test

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/multiformats/go-multiaddr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
)

type TestNode struct {
	api.FullNode
	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr
}

type TestStorageNode struct {
	api.StorageMiner
	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	MineOne func(context.Context, miner.MineReq) error
}

var PresealGenesis = -1

const GenesisPreseals = 2

// Options for setting up a mock storage miner
type StorageMiner struct {
	Full    int
	Preseal int
}

type OptionGenerator func([]TestNode) node.Option

// Options for setting up a mock full node
type FullNodeOpts struct {
	Lite bool            // run node in "lite" mode
	Opts OptionGenerator // generate dependency injection options
}

// APIBuilder is a function which is invoked in test suite to provide
// test nodes and networks
//
// fullOpts array defines options for each full node
// storage array defines storage nodes, numbers in the array specify full node
// index the storage node 'belongs' to
type APIBuilder func(t *testing.T, full []FullNodeOpts, storage []StorageMiner) ([]TestNode, []TestStorageNode)
type testSuite struct {
	makeNodes APIBuilder
}

// TestApis is the entry point to API test suite
func TestApis(t *testing.T, b APIBuilder) {
	ts := testSuite{
		makeNodes: b,
	}

	t.Run("version", ts.testVersion)
	t.Run("id", ts.testID)
	t.Run("testConnectTwo", ts.testConnectTwo)
	t.Run("testMining", ts.testMining)
	t.Run("testMiningReal", ts.testMiningReal)
	t.Run("testSearchMsg", ts.testSearchMsg)
}

func DefaultFullOpts(nFull int) []FullNodeOpts {
	full := make([]FullNodeOpts, nFull)
	for i := range full {
		full[i] = FullNodeOpts{
			Opts: func(nodes []TestNode) node.Option {
				return node.Options()
			},
		}
	}
	return full
}

var OneMiner = []StorageMiner{{Full: 0, Preseal: PresealGenesis}}
var OneFull = DefaultFullOpts(1)
var TwoFull = DefaultFullOpts(2)

var FullNodeWithUpgradeAt = func(upgradeHeight abi.ChainEpoch) FullNodeOpts {
	return FullNodeOpts{
		Opts: func(nodes []TestNode) node.Option {
			return node.Override(new(stmgr.UpgradeSchedule), stmgr.UpgradeSchedule{{
				// Skip directly to tape height so precommits work.
				Network:   network.Version5,
				Height:    upgradeHeight,
				Migration: stmgr.UpgradeActorsV2,
			}})
		},
	}
}

func (ts *testSuite) testVersion(t *testing.T) {
	build.RunningNodeType = build.NodeFull

	ctx := context.Background()
	apis, _ := ts.makeNodes(t, OneFull, OneMiner)
	api := apis[0]

	v, err := api.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, v.Version, build.BuildVersion)
}

func (ts *testSuite) testSearchMsg(t *testing.T) {
	apis, miners := ts.makeNodes(t, OneFull, OneMiner)

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
	bm := NewBlockMiner(ctx, t, miners[0], 100*time.Millisecond)
	bm.MineBlocks()
	defer bm.Stop()

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := api.StateWaitMsg(ctx, sm.Cid(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatal("did not successfully send message")
	}

	searchRes, err := api.StateSearchMsg(ctx, sm.Cid())
	if err != nil {
		t.Fatal(err)
	}

	if searchRes.TipSet != res.TipSet {
		t.Fatalf("search ts: %s, different from wait ts: %s", searchRes.TipSet, res.TipSet)
	}

}

func (ts *testSuite) testID(t *testing.T) {
	ctx := context.Background()
	apis, _ := ts.makeNodes(t, OneFull, OneMiner)
	api := apis[0]

	id, err := api.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assert.Regexp(t, "^12", id.Pretty())
}

func (ts *testSuite) testConnectTwo(t *testing.T) {
	ctx := context.Background()
	apis, _ := ts.makeNodes(t, TwoFull, OneMiner)

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
