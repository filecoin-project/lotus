package itests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/itests/kit"
)

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
	merged := kit.MergeFullNodes(nodes)

	var miner kit.TestMiner
	ens.Miner(&miner, merged, nopts...)

	ens.Start()

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
