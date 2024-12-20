package itests

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/node/impl/gasutils"
)

// calculateExpectations calculates the expected number of items to be included in the response
// of eth_feeHistory. It takes care of null rounds by finding the closet tipset with height
// smaller than startHeight, and then looks back at requestAmount of items. It also considers
// scenarios where there are not enough items to look back.
func calculateExpectations(tsHeights []int, requestAmount, startHeight int) (count, oldestHeight int) {
	latestIdx := sort.SearchInts(tsHeights, startHeight)
	// SearchInts returns the index of the number that's larger than the target if the target
	// doesn't exist. However, we're looking for the closet number that's smaller that the target
	for tsHeights[latestIdx] > startHeight {
		latestIdx--
	}
	cnt := requestAmount
	oldestIdx := latestIdx - requestAmount + 1
	if oldestIdx < 0 {
		cnt = latestIdx + 1
		oldestIdx = 0
	}
	return cnt, tsHeights[oldestIdx]
}

func TestEthFeeHistory(t *testing.T) {
	require := require.New(t)

	kit.QuietAllLogsExcept()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	miner := ens.InterconnectAll().BeginMining(blockTime)

	client.WaitTillChain(ctx, kit.HeightAtLeast(7))
	miner[0].InjectNulls(abi.ChainEpoch(5))

	// Wait for the network to create at least 20 tipsets
	client.WaitTillChain(ctx, kit.HeightAtLeast(20))
	for _, m := range miner {
		m.Pause()
	}

	ch, err := client.ChainNotify(ctx)
	require.NoError(err)

	// Wait for 5 seconds of inactivity
	func() {
		for {
			select {
			case <-ch:
				continue
			case <-time.After(5 * time.Second):
				return
			}
		}
	}()

	currTs, err := client.ChainHead(ctx)
	require.NoError(err)

	var tsHeights []int
	for currTs.Height() != 0 {
		tsHeights = append(tsHeights, int(currTs.Height()))
		currTs, err = client.ChainGetTipSet(ctx, currTs.Parents())
		require.NoError(err)
	}

	sort.Ints(tsHeights)

	// because of the deferred execution, the last tipset is not executed yet,
	// and the one before the last one is the last executed tipset,
	// which corresponds to the "latest" tag in EthGetBlockByNumber
	latestBlk := ethtypes.EthUint64(tsHeights[len(tsHeights)-2])
	blk, err := client.EthGetBlockByNumber(ctx, "latest", false)
	require.NoError(err)
	require.Equal(blk.Number, latestBlk)

	assertHistory := func(history *ethtypes.EthFeeHistory, requestAmount, startHeight int) {
		amount, oldest := calculateExpectations(tsHeights, requestAmount, startHeight)
		require.Equal(amount+1, len(history.BaseFeePerGas))
		require.Equal(amount, len(history.GasUsedRatio))
		require.Equal(ethtypes.EthUint64(oldest), history.OldestBlock)
	}

	history, err := client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "0x10"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 5, 16)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"5", "0x10"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 5, 16)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "latest"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 5, int(latestBlk))
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"0x10", "0x12"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 16, 18)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "0x10"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 5, 16)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "10"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 5, 10)
	require.Nil(history.Reward)

	// test when the requested number of blocks is longer than chain length
	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"0x30", "latest"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 48, int(latestBlk))
	require.Nil(history.Reward)

	// test when the requested number of blocks is longer than chain length
	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"0x30", "10"}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 48, 10)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "10", &[]float64{25, 50, 75}}),
	).Assert(require.NoError))
	require.NoError(err)
	assertHistory(&history, 5, 10)
	require.NotNil(history.Reward)
	require.Equal(5, len(*history.Reward))
	for _, arr := range *history.Reward {
		require.Equal(3, len(arr))
		for _, item := range arr {
			require.Equal(ethtypes.EthBigInt(types.NewInt(gasutils.MinGasPremium)), item)
		}
	}

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{1025, "10", &[]float64{25, 50, 75}}),
	).Assert(require.NoError))
	require.Error(err)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "10", &[]float64{75, 50}}),
	).Assert(require.NoError))
	require.Error(err)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "10", &[]float64{}}),
	).Assert(require.NoError))
	require.NoError(err)
}
