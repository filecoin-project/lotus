package itests

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
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
	var proxy api.FullNodeStruct
	cliutil.FullNodeProxy(nodes, &proxy)
	merged := *nodes[0]
	merged.FullNode = &proxy

	var miner kit.TestMiner
	ens.Miner(&miner, &merged, nopts...)

	ens.Start()

	t.Run("cancelled waits preserve the cause", func(t *testing.T) {
		cause := errors.New("caller stopped waiting")
		waitCtx, cancel := context.WithCancelCause(ctx)
		cancel(cause)

		_, err := merged.WaitMsgResult(waitCtx, cid.Undef, 0)
		require.ErrorIs(t, err, cause)
		_, err = merged.WaitTillChainOrError(waitCtx, func(*types.TipSet) bool { return false })
		require.ErrorIs(t, err, cause)
	})

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
