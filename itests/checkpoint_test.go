package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestCheckpointFork(t *testing.T) {
	ctx := context.Background()

	blocktime := 100 * time.Millisecond

	nopts := []kit.NodeOpt{kit.WithAllSubsystems(), kit.ThroughRPC()}
	var n1, n2 kit.TestFullNode
	var m1, m2 kit.TestMiner
	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&n1, nopts...).
		FullNode(&n2, nopts...).
		Miner(&m1, &n1, nopts...).
		Miner(&m2, &n1, nopts...).Start()

	// Start 2 of them.
	ens.InterconnectAll().BeginMining(blocktime)

	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(5)))
		cancel()
	}

	// Wait till both participate in a single tipset.
	var target *types.TipSet
	{
		// find the first tipset where two miners mine a block.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		target = n1.WaitTillChain(ctx, func(ts *types.TipSet) bool {
			return len(ts.Blocks()) == 2
		})
		cancel()
	}

	// Wait till we've moved on from that tipset.
	targetHeight := target.Height() + 10
	n1.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))
	n2.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))

	// Forcibly sync to this fork tipset.
	forkTs, err := types.NewTipSet(target.Blocks()[:1])
	require.NoError(t, err)
	require.NoError(t, n2.SyncCheckpoint(ctx, forkTs.Key()))
	require.NoError(t, n1.SyncCheckpoint(ctx, forkTs.Key()))

	// See if we can start making progress again!
	newHead := n1.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))
	forkTs2, err := n1.ChainGetTipSetByHeight(ctx, forkTs.Height(), newHead.Key())
	require.NoError(t, err)
	require.True(t, forkTs.Equals(forkTs2))
}
