package itests

import (
	"context"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const (
	DefaultBootsrapEpoch = 30
	DefaultFinality      = 5
	DefaultEpochBuffer   = 30
)

type testEnv struct {
	minerFullNode *kit.TestFullNode
	observer      *kit.TestFullNode
	ms            *manifest.ManifestSender
	m             manifest.Manifest
}

// Test that checks that F3 is enabled successfully,
// and miners are able to bootstrap and make progress
func TestF3_Enabled(t *testing.T) {

	lvl, err := logging.LevelFromString("error")
	if err != nil {
		panic(err)
	}
	logging.SetAllLoggers(lvl)

	err = logging.SetLogLevel("f3", "debug")
	if err != nil {
		panic(err)
	}
	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)

	ctx := context.Background()

	waitTillF3Instance(t, ctx, e.minerFullNode, 3, 20*time.Second)
}

func TestF3_Rebootstrap(t *testing.T) {
	lvl, err := logging.LevelFromString("error")
	if err != nil {
		panic(err)
	}
	logging.SetAllLoggers(lvl)

	err = logging.SetLogLevel("f3", "debug")
	if err != nil {
		panic(err)
	}

	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)

	ctx := context.Background()
	prevManifest, err := e.minerFullNode.F3GetManifest(ctx)
	require.NoError(t, err)

	newInstance := uint64(3)
	waitTillF3Instance(t, ctx, e.minerFullNode, newInstance, 20*time.Second)
	prevHead, err := e.minerFullNode.ChainHead(ctx)
	require.NoError(t, err)

	e.m.Sequence++
	e.m.BootstrapEpoch = 50
	e.m.ReBootstrap = true
	e.ms.UpdateManifest(e.m)

	waitTillManifestChange(t, ctx, e.minerFullNode, prevManifest, 20*time.Second)

	newHead, err := e.minerFullNode.ChainHead(ctx)
	require.NoError(t, err)

	e.minerFullNode.WaitTillChain(ctx, hasBootstrapped(abi.ChainEpoch(e.m.BootstrapEpoch), prevHead.Height()))
	require.NotEqual(t, prevHead.Key(), newHead.Key())
}

func TestF3_PauseAndRebootstrap(t *testing.T) {
	// TODO:
	///
	///

	// lvl, err := logging.LevelFromString("error")
	// if err != nil {
	// 	panic(err)
	// }
	// logging.SetAllLoggers(lvl)

	// err = logging.SetLogLevel("f3", "debug")
	// if err != nil {
	// 	panic(err)
	// }

	// blocktime := 100 * time.Millisecond
	// e := setup(t, blocktime)

	// ctx := context.Background()
	// prevManifest, err := e.minerFullNode.F3GetManifest(ctx)
	// require.NoError(t, err)

	// newInstance := uint64(3)
	// waitTillF3Instance(t, ctx, e.minerFullNode, newInstance, 20*time.Second)
	// prevHead, err := e.minerFullNode.ChainHead(ctx)
	// require.NoError(t, err)

	// e.m.Sequence++
	// e.m.BootstrapEpoch = 50
	// e.m.ReBootstrap = true
	// e.ms.UpdateManifest(e.m)

	// waitTillManifestChange(t, ctx, e.minerFullNode, prevManifest, 20*time.Second)

	// newHead, err := e.minerFullNode.ChainHead(ctx)
	// require.NoError(t, err)

	// e.minerFullNode.WaitTillChain(ctx, hasBootstrapped(abi.ChainEpoch(e.m.BootstrapEpoch), prevHead.Height()))
	// require.NotEqual(t, prevHead.Key(), newHead.Key())
}

func hasBootstrapped(bootstrapEpoch, prevHead abi.ChainEpoch) kit.ChainPredicate {
	return func(ts *types.TipSet) bool {
		return ts.Height() >= bootstrapEpoch && ts.Height() < prevHead
	}
}

// TODO:
// Sees that F3 makes progress with the chain (this one I have it but will have to bump the version).
// Dynamic manifest successfully rebootstraps.
// Dynamic manifest pauses F3 and the rebootstraps.

func waitTillF3Instance(t *testing.T, ctx context.Context, n *kit.TestFullNode, i uint64, timeout time.Duration) {
	require.Eventually(t, func() bool {
		c, err := n.F3GetLatestCertificate(ctx)
		require.NoError(t, err)
		if c != nil {
			return c.GPBFTInstance >= i
		}
		return false

	}, timeout, 100*time.Millisecond)

}

func waitTillManifestChange(t *testing.T, ctx context.Context, n *kit.TestFullNode, prevManifest *manifest.Manifest, timeout time.Duration) {
	require.Eventually(t, func() bool {
		m, err := n.F3GetManifest(ctx)
		require.NoError(t, err)
		v1, err := prevManifest.Version()
		require.NoError(t, err)
		v2, err := m.Version()
		require.NoError(t, err)
		return v1 != v2

	}, timeout, 100*time.Millisecond)

}

func TestF3_Detection(t *testing.T) {}

// Test dynamic parameter adjustement works.
func TestF3_DynamicManifest(t *testing.T) {}

// Fallback mechanism for when F3 fails
func TestF3_Fallback(t *testing.T) {}

// Setup creates a new F3-enabled network with two miners and two full-nodes
//
// The first node returned by the function is directly connected to a miner,
// and the second full-node is an observer that is not directly connected to
// a miner. The last return value is the manifest sender for the network.
func setup(t *testing.T, blocktime time.Duration) testEnv {
	ctx := context.Background()

	// create manifest host first to get the manifest ID to setup F3
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/udp/0/quic-v1"))
	require.NoError(t, err)
	build.ManifestServerID = h.ID().String()

	f3Opts := kit.F3Enabled(DefaultBootsrapEpoch, blocktime, DefaultFinality)
	// miner is connected to the first node, and we want to observe the chain
	// from the second node.
	n1, m1, m2, ens := kit.EnsembleOneTwoF3(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		f3Opts,
	)
	ens.InterconnectAll().BeginMining(blocktime)
	nn, err := n1.StateNetworkName(context.Background())
	require.NoError(t, err)

	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(5)))
		cancel()
	}

	var n2 kit.TestFullNode
	ens.FullNode(&n2, kit.ThroughRPC(), f3Opts).Start().Connect(n2, n1)

	{
		// find the first tipset where all miners mined a block.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		n2.WaitTillChain(ctx, kit.BlocksMinedByAll(m1.ActorAddr, m2.ActorAddr))
		cancel()
	}

	head, err := n2.ChainHead(context.Background())
	require.NoError(t, err)

	// wait for the chain to be bootstrapped
	n2.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+DefaultBootsrapEpoch))

	// create manifest sender and connect to full-nodes
	ms := newManifestSender(t, context.Background(), h, nn, blocktime)
	err = n1.NetConnect(ctx, ms.PeerInfo())
	require.NoError(t, err)
	err = n2.NetConnect(ctx, ms.PeerInfo())
	require.NoError(t, err)
	go ms.Start(ctx)

	return testEnv{n1, &n2, ms, lf3.NewManifest(nn)}
}

func newManifestSender(t *testing.T, ctx context.Context, h host.Host, nn dtypes.NetworkName, senderTimeout time.Duration) *manifest.ManifestSender {

	ps, err := pubsub.NewGossipSub(ctx, h)
	require.NoError(t, err)

	m := lf3.NewManifest(nn)
	ms, err := manifest.NewManifestSender(h, ps, m, senderTimeout)
	require.NoError(t, err)
	return ms
}
