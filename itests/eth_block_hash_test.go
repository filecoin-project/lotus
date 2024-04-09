package itests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestEthBlockHashesCorrect_MultiBlockTipset validates that blocks retrieved through
// EthGetBlockByNumber are identical to blocks retrieved through
// EthGetBlockByHash, when using the block hash returned by the former.
//
// Specifically, it checks the system behaves correctly with multiblock tipsets.
//
// Catches regressions around https://github.com/filecoin-project/lotus/issues/10061.
func TestEthBlockHashesCorrect_MultiBlockTipset(t *testing.T) {
	// miner is connected to the first node, and we want to observe the chain
	// from the second node.
	blocktime := 100 * time.Millisecond
	n1, m1, m2, ens := kit.EnsembleOneTwo(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(5)))
		cancel()
	}

	var n2 kit.TestFullNode
	ens.FullNode(&n2, kit.ThroughRPC()).Start().Connect(n2, n1)

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
	n2.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+50))

	tsk := head.Key()
	for i := 1; i <= int(head.Height()); i++ {
		hex := fmt.Sprintf("0x%x", i)

		ts, err := n2.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(i), tsk)
		require.NoError(t, err)

		ethBlockA, err := n2.EthGetBlockByNumber(ctx, hex, true)
		// Cannot use static ErrFullRound error for comparison since it gets reserialized as a JSON RPC error.
		if err != nil && strings.Contains(err.Error(), "null round") {
			require.Less(t, ts.Height(), abi.ChainEpoch(i), "did not expect a tipset at epoch %d", i)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, ts.Height(), abi.ChainEpoch(i), "expected a tipset at epoch %i", i)

		ethBlockB, err := n2.EthGetBlockByHash(ctx, ethBlockA.Hash, true)
		require.NoError(t, err)

		require.Equal(t, ethBlockA, ethBlockB)

		numBlocks := len(ts.Blocks())
		expGasLimit := ethtypes.EthUint64(int64(numBlocks) * build.BlockGasLimit)
		require.Equal(t, expGasLimit, ethBlockB.GasLimit)
	}
}
