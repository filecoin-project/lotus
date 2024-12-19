package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
)

func TestChainCheckpoint(t *testing.T) {
	ctx := context.Background()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}
	cg.AdvanceState = false

	// Let the first miner mine some blocks.
	last := cg.CurTipset.TipSet()
	for i := 0; i < 4; i++ {
		ts, err := cg.NextTipSetFromMiners(last, cg.Miners[:1], 0)
		require.NoError(t, err)

		last = ts.TipSet.TipSet()
	}

	cs := cg.ChainStore()

	checkpoint := last
	checkpointParents, err := cs.GetTipSetFromKey(ctx, checkpoint.Parents())
	require.NoError(t, err)

	// Set the head to the block before the checkpoint.
	err = cs.SetHead(ctx, checkpointParents)
	require.NoError(t, err)

	// Verify it worked.
	head := cs.GetHeaviestTipSet()
	require.True(t, head.Equals(checkpointParents))

	// Checkpoint into the future.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.NoError(t, err)

	// And verify that it worked.
	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(checkpoint))

	// Let the second miner mine a fork
	last = checkpointParents
	for i := 0; i < 4; i++ {
		ts, err := cg.NextTipSetFromMiners(last, cg.Miners[1:], 0)
		require.NoError(t, err)

		last = ts.TipSet.TipSet()
	}

	// See if the chain will take the fork, it shouldn't.
	err = cs.RefreshHeaviestTipSet(context.Background(), last.Height())
	require.NoError(t, err)
	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(checkpoint))

	// Remove the checkpoint.
	err = cs.RemoveCheckpoint(ctx)
	require.NoError(t, err)

	// Now switch to the other fork.
	err = cs.RefreshHeaviestTipSet(context.Background(), last.Height())
	require.NoError(t, err)
	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(last))

	// We should switch back if we checkpoint again.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.NoError(t, err)

	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(checkpoint))

	// Now extend the fork 900 epochs into the future.
	for i := 0; i < int(policy.ChainFinality)+10; i++ {
		ts, err := cg.NextTipSetFromMiners(last, cg.Miners[1:], 0)
		require.NoError(t, err)

		last = ts.TipSet.TipSet()
	}

	// Try to re-checkpoint to the long fork. This will work because we only have to revert a
	// single epoch.
	err = cs.SetCheckpoint(ctx, last)
	require.NoError(t, err)

	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(last))

	// Now try to go back to the checkpoint. This should fail because it's too far in the past
	// on the wrong fork.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.Error(t, err)

	// Now extend the checkpoint chain to the same tipset.
	for checkpoint.Height() < last.Height() {
		ts, err := cg.NextTipSetFromMiners(checkpoint, cg.Miners[1:], 0)
		require.NoError(t, err)

		checkpoint = ts.TipSet.TipSet()
	}

	// We should still refuse to switch.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.Error(t, err)

	// But it should be possible to set a checkpoint on a common chain
	err = cs.SetCheckpoint(ctx, checkpointParents)
	require.NoError(t, err)
}
