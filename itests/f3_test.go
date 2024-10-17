package itests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
)

const (
	DefaultBootstrapEpoch                   = 20
	DefaultFinality                         = 5
	BaseNetworkName       gpbft.NetworkName = "test"
)

type testEnv struct {
	minerFullNodes []*kit.TestFullNode
	// observer currently not use but may come handy to test certificate exchanges
	ms      *manifest.ManifestSender
	m       *manifest.Manifest
	t       *testing.T
	testCtx context.Context
	debug   bool
}

// Test that checks that F3 is enabled successfully,
// and miners are able to bootstrap and make progress
func TestF3_Enabled(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)

	e.waitTillF3Instance(lf3.ParticipationLeaseTerm+1, 40*time.Second)
}

// Test that checks that F3 can be rebootsrapped by changing the manifest
func TestF3_Rebootstrap(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)
	n := e.minerFullNodes[0]

	newInstance := uint64(2)
	e.waitTillF3Instance(newInstance, 20*time.Second)

	prevCert, err := n.F3GetCertificate(e.testCtx, newInstance)
	require.NoError(t, err)

	cpy := *e.m
	cpy.BootstrapEpoch = 25
	cpy.NetworkName = BaseNetworkName + "/2"
	e.ms.UpdateManifest(&cpy)

	newManifest := e.waitTillManifestChange(&cpy, 20*time.Second)
	require.True(t, newManifest.Equal(&cpy))
	e.waitTillF3Rebootstrap(20 * time.Second)
	e.waitTillF3Instance(prevCert.GPBFTInstance+1, 20*time.Second)
}

// Tests that pause/resume and rebootstrapping F3 works
func TestF3_PauseAndRebootstrap(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 100 * time.Millisecond
	e := setup(t, blocktime)

	newInstance := uint64(2)
	e.waitTillF3Instance(newInstance, 20*time.Second)

	origManifest := *e.m
	pausedManifest := origManifest
	pausedManifest.Pause = true
	e.ms.UpdateManifest(&pausedManifest)
	e.waitTillF3Pauses(30 * time.Second)

	e.ms.UpdateManifest(&origManifest)
	e.waitTillF3Runs(30 * time.Second)

	cpy := *e.m
	cpy.NetworkName = BaseNetworkName + "/2"
	cpy.BootstrapEpoch = 25
	e.ms.UpdateManifest(&cpy)

	e.waitTillManifestChange(&cpy, 20*time.Second)
	e.waitTillF3Rebootstrap(20 * time.Second)
}

// Tests that pause/resume and rebootstrapping F3 works
func TestF3_Bootstrap(t *testing.T) {
	kit.QuietMiningLogs()

	var bootstrapEpoch abi.ChainEpoch = 50
	blocktime := 100 * time.Millisecond
	staticManif := lf3.NewManifest(BaseNetworkName, DefaultFinality, bootstrapEpoch, blocktime, cid.Undef)
	dynamicManif := *staticManif
	dynamicManif.BootstrapEpoch = 5
	dynamicManif.EC.Finalize = false
	dynamicManif.NetworkName = BaseNetworkName + "/1"

	e := setupWithStaticManifest(t, staticManif, true)
	e.ms.UpdateManifest(&dynamicManif)
	e.waitTillManifestChange(&dynamicManif, 20*time.Second)
	e.waitTillF3Instance(2, 20*time.Second)
	e.waitTillManifestChange(staticManif, 20*time.Second)
	e.waitTillF3Instance(2, 20*time.Second)

	// Try to switch back, we should ignore the manifest update.
	e.ms.UpdateManifest(&dynamicManif)
	time.Sleep(time.Second)
	for _, n := range e.minerFullNodes {
		m, err := n.F3GetManifest(e.testCtx)
		require.NoError(e.t, err)
		require.True(t, m.Equal(staticManif))
	}
}

func (e *testEnv) waitTillF3Rebootstrap(timeout time.Duration) {
	e.waitFor(func(n *kit.TestFullNode) bool {
		// the prev epoch yet, check if we already bootstrapped and from
		// the right epoch
		cert, err := n.F3GetCertificate(e.testCtx, 0)
		if err != nil || cert == nil {
			return false
		}
		m, err := n.F3GetManifest(e.testCtx)
		require.NoError(e.t, err)

		// Find the first non-null block at or before the target height, that's the bootstrap block.
		targetEpoch := m.BootstrapEpoch - m.EC.Finality
		ts, err := n.ChainGetTipSetByHeight(e.testCtx, abi.ChainEpoch(targetEpoch), types.EmptyTSK)
		if err != nil {
			return false
		}

		return cert.ECChain.Base().Epoch == int64(ts.Height())
	}, timeout)
}

func (e *testEnv) waitTillF3Pauses(timeout time.Duration) {
	e.waitFor(func(n *kit.TestFullNode) bool {
		r, err := n.F3IsRunning(e.testCtx)
		require.NoError(e.t, err)
		return !r
	}, timeout)
}

func (e *testEnv) waitTillF3Runs(timeout time.Duration) {
	e.waitFor(func(n *kit.TestFullNode) bool {
		r, err := n.F3IsRunning(e.testCtx)
		require.NoError(e.t, err)
		return r
	}, timeout)
}

func (e *testEnv) waitTillF3Instance(i uint64, timeout time.Duration) {
	e.waitFor(func(n *kit.TestFullNode) bool {
		c, err := n.F3GetLatestCertificate(e.testCtx)
		if err != nil {
			require.ErrorContains(e.t, err, "F3 is not running")
			return false
		}
		return c != nil && c.GPBFTInstance >= i
	}, timeout)
}

func (e *testEnv) waitTillManifestChange(newManifest *manifest.Manifest, timeout time.Duration) (m *manifest.Manifest) {
	e.waitFor(func(n *kit.TestFullNode) bool {
		var err error
		m, err = n.F3GetManifest(e.testCtx)
		require.NoError(e.t, err)
		return newManifest.Equal(m)
	}, timeout)
	return m

}
func (e *testEnv) waitFor(f func(n *kit.TestFullNode) bool, timeout time.Duration) {
	e.t.Helper()
	require.Eventually(e.t, func() bool {
		e.t.Helper()
		defer func() {
			if e.debug {
				var wg sync.WaitGroup
				printProgress := func(index int, n *kit.TestFullNode) {
					defer wg.Done()
					if progress, err := n.F3GetProgress(e.testCtx); err != nil {
						e.t.Logf("Node #%d progress: err: %v", index, err)
					} else {
						e.t.Logf("Node #%d progress: %v", index, progress)
					}
				}
				for i, n := range e.minerFullNodes {
					wg.Add(1)
					go printProgress(i, n)
				}
				wg.Wait()
			}
		}()
		for _, n := range e.minerFullNodes {
			if !f(n) {
				return false
			}
		}
		return true
	}, timeout, 100*time.Millisecond)
}

// Setup creates a new F3-enabled network with two miners and two full-nodes
//
// The first node returned by the function is directly connected to a miner,
// and the second full-node is an observer that is not directly connected to
// a miner. The last return value is the manifest sender for the network.
func setup(t *testing.T, blocktime time.Duration) *testEnv {
	return setupWithStaticManifest(t, newTestManifest(blocktime), false)
}

func newTestManifest(blocktime time.Duration) *manifest.Manifest {
	return &manifest.Manifest{
		ProtocolVersion:   manifest.VersionCapability,
		BootstrapEpoch:    DefaultBootstrapEpoch,
		NetworkName:       BaseNetworkName + "/1",
		InitialPowerTable: cid.Undef,
		CommitteeLookback: manifest.DefaultCommitteeLookback,
		CatchUpAlignment:  blocktime / 2,
		Gpbft: manifest.GpbftConfig{
			// Use smaller time intervals for more responsive test progress/assertion.
			Delta:                      250 * time.Millisecond,
			DeltaBackOffExponent:       1.3,
			MaxLookaheadRounds:         5,
			RebroadcastBackoffBase:     500 * time.Millisecond,
			RebroadcastBackoffSpread:   0.1,
			RebroadcastBackoffExponent: 1.3,
			RebroadcastBackoffMax:      1 * time.Second,
		},
		EC: manifest.EcConfig{
			Period:                   blocktime,
			Finality:                 DefaultFinality,
			DelayMultiplier:          manifest.DefaultEcConfig.DelayMultiplier,
			BaseDecisionBackoffTable: manifest.DefaultEcConfig.BaseDecisionBackoffTable,
			HeadLookback:             0,
			Finalize:                 true,
		},
		CertificateExchange: manifest.CxConfig{
			ClientRequestTimeout: manifest.DefaultCxConfig.ClientRequestTimeout,
			ServerRequestTimeout: manifest.DefaultCxConfig.ServerRequestTimeout,
			MinimumPollInterval:  blocktime,
			MaximumPollInterval:  4 * blocktime,
		},
	}
}

func setupWithStaticManifest(t *testing.T, manif *manifest.Manifest, testBootstrap bool) *testEnv {
	ctx, stopServices := context.WithCancel(context.Background())
	errgrp, ctx := errgroup.WithContext(ctx)

	blocktime := manif.EC.Period

	t.Cleanup(func() {
		stopServices()
		require.NoError(t, errgrp.Wait())
	})

	// create manifest host first to get the manifest ID to setup F3
	manifestServerHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/udp/0/quic-v1"))
	require.NoError(t, err)

	cfg := &lf3.Config{
		BaseNetworkName:          BaseNetworkName,
		StaticManifest:           manif,
		DynamicManifestProvider:  manifestServerHost.ID(),
		PrioritizeStaticManifest: testBootstrap,
		AllowDynamicFinalize:     !testBootstrap,
	}

	f3NOpt := kit.F3Enabled(cfg)
	f3MOpt := kit.ConstructorOpts(node.Override(node.F3Participation, modules.F3Participation))

	var (
		n1, n2, n3     kit.TestFullNode
		m1, m2, m3, m4 kit.TestMiner
	)

	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&n1, kit.WithAllSubsystems(), f3NOpt).
		FullNode(&n2, kit.WithAllSubsystems(), f3NOpt).
		FullNode(&n3, kit.WithAllSubsystems(), f3NOpt).
		Miner(&m1, &n1, kit.WithAllSubsystems(), f3MOpt).
		Miner(&m2, &n2, kit.WithAllSubsystems(), f3MOpt).
		Miner(&m3, &n3, kit.WithAllSubsystems(), f3MOpt).
		Miner(&m4, &n3, kit.WithAllSubsystems(), f3MOpt).
		Start()

	ens.InterconnectAll().BeginMining(blocktime)

	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(5)))
		cancel()
	}

	e := &testEnv{m: manif, t: t, testCtx: ctx}
	// in case we want to use more full-nodes in the future
	e.minerFullNodes = []*kit.TestFullNode{&n1, &n2, &n3}

	// create manifest sender and connect to full-nodes
	e.ms = e.newManifestSender(context.Background(), t, manifestServerHost, blocktime)
	for _, n := range e.minerFullNodes {
		err = n.NetConnect(ctx, e.ms.PeerInfo())
		require.NoError(t, err)
	}
	errgrp.Go(func() error {
		defer func() {
			require.NoError(t, manifestServerHost.Close())
		}()
		return e.ms.Run(ctx)
	})

	return e
}

func (e *testEnv) newManifestSender(ctx context.Context, t *testing.T, h host.Host, senderTimeout time.Duration) *manifest.ManifestSender {
	ps, err := pubsub.NewGossipSub(ctx, h)
	require.NoError(t, err)

	ms, err := manifest.NewManifestSender(ctx, h, ps, e.m, senderTimeout)
	require.NoError(t, err)
	return ms
}
