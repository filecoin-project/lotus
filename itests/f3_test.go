package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestF3_Progress(t *testing.T) {
	blocktime := 100 * time.Millisecond
	f3Opts := kit.F3Enabled(10, blocktime, 5)
	// miner is connected to the first node, and we want to observe the chain
	// from the second node.
	n1, m1, m2, ens := kit.EnsembleOneTwoF3(t,
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

	ctx := context.Background()

	// let the chain run a little bit longer to minimise the chance of reorgs
	n2.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+30))

	c, err := n2.F3GetLatestCertificate(ctx)
	require.NoError(t, err)
	require.True(t, c.GPBFTInstance > 0)
}

func TestF3_Detection(t *testing.T) {}

// Test dynamic parameter adjustement works.
func TestF3_DynamicManifest(t *testing.T) {}

// Fallback mechanism for when F3 fails
func TestF3_DynamicManifest(t *testing.T) {}
