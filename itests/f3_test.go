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

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/itests/kit"
)

const (
	DefaultBootsrapEpoch = 20
	DefaultFinality      = 5
	F3FriendlyDebugLogs  = true
)

type testEnv struct {
	minerFullNodes []*kit.TestFullNode
	// observer currently not use but may come handy to test certificate exchanges
	observer *kit.TestFullNode
	ms       *manifest.ManifestSender
	m        *manifest.Manifest
}

// Test that checks that F3 is enabled successfully,
// and miners are able to bootstrap and make progress
func TestF3_Enabled(t *testing.T) {
	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)

	ctx := context.Background()

	e.waitTillF3Instance(t, ctx, 3, 20*time.Second)
}

// Test that checks that F3 can be rebootsrapped by changing the manifest
func TestF3_Rebootstrap(t *testing.T) {
	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)
	n := e.minerFullNodes[0]

	ctx := context.Background()
	prevManifest, err := n.F3GetManifest(ctx)
	require.NoError(t, err)

	newInstance := uint64(3)
	e.waitTillF3Instance(t, ctx, newInstance, 20*time.Second)

	prevCert, err := n.F3GetCertificate(ctx, newInstance)
	require.NoError(t, err)

	cpy := *e.m
	cpy.BootstrapEpoch = 30
	e.ms.UpdateManifest(&cpy)

	// wait for manifest change, wait for the rebootstrap,
	// and check that the chain finalised after rebootsrapping
	// is different from the previous one.
	e.waitTillManifestChange(t, ctx, prevManifest, 20*time.Second)
	e.waitTillF3Rebootstrap(t, ctx, prevCert, 20*time.Second)
	e.waitTillF3Instance(t, ctx, prevCert.GPBFTInstance+1, 20*time.Second)

	curr, err := n.F3GetCertificate(ctx, prevCert.GPBFTInstance)
	require.NoError(t, err)
	require.NotNil(t, curr)
	require.NotEqual(t, prevCert.ECChain, curr.ECChain)

	// Check that we are bootstrapping from the right epoch
	curr, err = n.F3GetCertificate(ctx, 0)
	require.NoError(t, err)
	require.NotNil(t, curr)
	require.True(t, curr.ECChain[0].Epoch == cpy.BootstrapEpoch-int64(build.F3Finality))
}

// Tests that pause/resume and rebootstrapping F3 works
func TestF3_PauseAndRebootstrap(t *testing.T) {
	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)
	n := e.minerFullNodes[0]

	ctx := context.Background()
	prevManifest, err := n.F3GetManifest(ctx)
	require.NoError(t, err)

	newInstance := uint64(3)
	e.waitTillF3Instance(t, ctx, newInstance, 20*time.Second)

	e.ms.Pause()
	e.waitTillF3Pauses(t, ctx, 20*time.Second)

	e.ms.Resume()
	e.waitTillF3Runs(t, ctx, 20*time.Second)

	cpy := *e.m
	cpy.BootstrapEpoch = 30
	e.ms.UpdateManifest(&cpy)

	e.waitTillManifestChange(t, ctx, prevManifest, 20*time.Second)
}

func (e *testEnv) waitTillF3Rebootstrap(t *testing.T, ctx context.Context, prev *certs.FinalityCertificate, timeout time.Duration) {
	e.waitFor(t, func(n *kit.TestFullNode) bool {
		curr, err := n.F3GetLatestCertificate(ctx)
		require.NoError(t, err)
		return curr != nil && curr.GPBFTInstance > 0 && curr.GPBFTInstance < prev.GPBFTInstance
	}, timeout)
}

func (e *testEnv) waitTillF3Pauses(t *testing.T, ctx context.Context, timeout time.Duration) {
	e.waitFor(t, func(n *kit.TestFullNode) bool {
		r, err := n.F3IsRunning(ctx)
		require.NoError(t, err)
		return !r
	}, timeout)
}

func (e *testEnv) waitTillF3Runs(t *testing.T, ctx context.Context, timeout time.Duration) {
	e.waitFor(t, func(n *kit.TestFullNode) bool {
		r, err := n.F3IsRunning(ctx)
		require.NoError(t, err)
		return r
	}, timeout)
}

func (e *testEnv) waitTillF3Instance(t *testing.T, ctx context.Context, i uint64, timeout time.Duration) {
	e.waitFor(t, func(n *kit.TestFullNode) bool {
		c, err := n.F3GetLatestCertificate(ctx)
		require.NoError(t, err)
		if c != nil {
			return c.GPBFTInstance >= i
		}
		return false

	}, timeout)

}

func (e *testEnv) waitTillManifestChange(t *testing.T, ctx context.Context, prevManifest *manifest.Manifest, timeout time.Duration) {
	e.waitFor(t, func(n *kit.TestFullNode) bool {
		m, err := n.F3GetManifest(ctx)
		require.NoError(t, err)
		v1, err := prevManifest.Version()
		require.NoError(t, err)
		v2, err := m.Version()
		require.NoError(t, err)
		return v1 != v2

	}, timeout)

}
func (e *testEnv) waitFor(t *testing.T, f func(n *kit.TestFullNode) bool, timeout time.Duration) {
	require.Eventually(t, func() bool {
		reached := 0

		for _, n := range e.minerFullNodes {
			if f(n) {
				reached++
			}
			if reached == len(e.minerFullNodes) {
				return true
			}
		}
		return false
	}, timeout, 500*time.Millisecond)
}

// Setup creates a new F3-enabled network with two miners and two full-nodes
//
// The first node returned by the function is directly connected to a miner,
// and the second full-node is an observer that is not directly connected to
// a miner. The last return value is the manifest sender for the network.
func setup(t *testing.T, blocktime time.Duration) testEnv {
	setUpF3DebugLogging()
	ctx := context.Background()

	// create manifest host first to get the manifest ID to setup F3
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/0.0.0.0/udp/0/quic-v1"))
	require.NoError(t, err)
	build.ManifestServerID = h.ID().String()

	f3Opts := kit.F3Enabled(DefaultBootsrapEpoch, blocktime, DefaultFinality)

	n1, m1, m2, ens := kit.EnsembleF3(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		f3Opts,
	)
	ens.InterconnectAll().BeginMining(blocktime)

	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(5)))
		cancel()
	}

	var obs kit.TestFullNode
	ens.FullNode(&obs, kit.ThroughRPC(), f3Opts).Start().Connect(obs, n1)

	{
		// find the first tipset where all miners mined a block.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		obs.WaitTillChain(ctx, kit.BlocksMinedByAll(m1.ActorAddr, m2.ActorAddr))
		cancel()
	}

	nn, err := n1.StateNetworkName(context.Background())
	require.NoError(t, err)

	e := testEnv{m: lf3.NewManifest(nn)}
	// in case we want to use more full-nodes in the future
	e.minerFullNodes = []*kit.TestFullNode{n1}
	e.observer = &obs

	// create manifest sender and connect to full-nodes
	e.ms = e.newManifestSender(t, context.Background(), h, blocktime)
	for _, n := range e.minerFullNodes {
		err = n.NetConnect(ctx, e.ms.PeerInfo())
		require.NoError(t, err)
	}
	err = obs.NetConnect(ctx, e.ms.PeerInfo())
	require.NoError(t, err)

	go e.ms.Run(ctx)
	return e
}

func (e *testEnv) newManifestSender(t *testing.T, ctx context.Context, h host.Host, senderTimeout time.Duration) *manifest.ManifestSender {
	ps, err := pubsub.NewGossipSub(ctx, h)
	require.NoError(t, err)

	ms, err := manifest.NewManifestSender(h, ps, e.m, senderTimeout)
	require.NoError(t, err)
	return ms
}

// This is a convenient function to set up a combination of logging
// levels that reduces the noise and eases the debugging of F3-related
// issues.
func setUpF3DebugLogging() {
	if F3FriendlyDebugLogs {

		lvl, err := logging.LevelFromString("error")
		if err != nil {
			panic(err)
		}
		logging.SetAllLoggers(lvl)

		err = logging.SetLogLevel("f3", "debug")
		if err != nil {
			panic(err)
		}
	}
}
