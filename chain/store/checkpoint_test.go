// stm: #unit
package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/gen"
)

func TestChainCheckpoint(t *testing.T) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_FROM_MINERS_001
	//stm: @CHAIN_STORE_GET_TIPSET_FROM_KEY_001, @CHAIN_STORE_SET_HEAD_001, @CHAIN_STORE_GET_HEAVIEST_TIPSET_001
	//stm: @CHAIN_STORE_SET_CHECKPOINT_001, @CHAIN_STORE_MAYBE_TAKE_HEAVIER_TIPSET_001, @CHAIN_STORE_REMOVE_CHECKPOINT_001
	ctx := context.Background()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

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

	// Try to set the checkpoint in the future, it should fail.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.Error(t, err)

	// Then move the head back.
	err = cs.SetHead(ctx, checkpoint)
	require.NoError(t, err)

	// Verify it worked.
	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(checkpoint))

	// And checkpoint it.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.NoError(t, err)

	// Let the second miner miner mine a fork
	last = checkpointParents
	for i := 0; i < 4; i++ {
		ts, err := cg.NextTipSetFromMiners(last, cg.Miners[1:], 0)
		require.NoError(t, err)

		last = ts.TipSet.TipSet()
	}

	// See if the chain will take the fork, it shouldn't.
	err = cs.MaybeTakeHeavierTipSet(context.Background(), last)
	require.NoError(t, err)
	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(checkpoint))

	// Remove the checkpoint.
	err = cs.RemoveCheckpoint(ctx)
	require.NoError(t, err)

	// Now switch to the other fork.
	err = cs.MaybeTakeHeavierTipSet(context.Background(), last)
	require.NoError(t, err)
	head = cs.GetHeaviestTipSet()
	require.True(t, head.Equals(last))

	// Setting a checkpoint on the other fork should fail.
	err = cs.SetCheckpoint(ctx, checkpoint)
	require.Error(t, err)

	// Setting a checkpoint on this fork should succeed.
	err = cs.SetCheckpoint(ctx, checkpointParents)
	require.NoError(t, err)
}
