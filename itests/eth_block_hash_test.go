package itests

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
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
		n1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(10)))
		cancel()
	}

	var n2 kit.TestFullNode
	ens.FullNode(&n2, kit.ThroughRPC()).Start().Connect(n2, n1)

	var head *types.TipSet
	{
		// find the first tipset where all miners mined a block.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		head = n2.WaitTillChain(ctx, kit.BlocksMinedByAll(m1.ActorAddr, m2.ActorAddr))
		cancel()
	}

	ctx := context.Background()

	// let the chain run a little bit longer to minimise the chance of reorgs
	n2.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+50))

	for i := 1; i <= int(head.Height()); i++ {
		hex := fmt.Sprintf("0x%x", i)

		// Skip null rounds. EthGetBlockByNumber resolves the block number against the current
		// heaviest head, so look up the height the same way here.
		curHead, err := n2.ChainHead(ctx)
		require.NoError(t, err)
		heightTs, err := n2.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(i), curHead.Key())
		require.NoError(t, err)
		if heightTs.Height() != abi.ChainEpoch(i) { // null round
			continue
		}

		ethBlockA, err := n2.EthGetBlockByNumber(ctx, hex, true)
		require.NoError(t, err)

		// Derive the tipset from ethBlockA.Hash to avoid a reorg race between calls.
		// EthBlock.Hash == EthHashFromCid(ts.Key().Cid()), so this round-trips to the exact tipset.
		tipsetKeyRaw, err := n2.ChainReadObj(ctx, ethBlockA.Hash.ToCid())
		require.NoError(t, err)
		var tsk types.TipSetKey
		require.NoError(t, tsk.UnmarshalCBOR(bytes.NewReader(tipsetKeyRaw)))
		ts, err := n2.ChainGetTipSet(ctx, tsk)
		require.NoError(t, err)

		require.EqualValues(t, ts.Height(), ethBlockA.Number)

		ethBlockB, err := n2.EthGetBlockByHash(ctx, ethBlockA.Hash, true)
		require.NoError(t, err)

		require.Equal(t, ethBlockA, ethBlockB)

		numBlocks := len(ts.Blocks())
		expGasLimit := ethtypes.EthUint64(int64(numBlocks) * buildconstants.BlockGasLimit)
		require.Equal(t, expGasLimit, ethBlockB.GasLimit, "expected gas limit to be %d for %d blocks", expGasLimit, numBlocks)
	}
}
