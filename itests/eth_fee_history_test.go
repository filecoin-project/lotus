package itests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/node/impl/full"
)

func TestEthFeeHistory(t *testing.T) {
	require := require.New(t)

	kit.QuietAllLogsExcept()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	miner := ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Wait for the network to create 20 blocks
	client.WaitTillChain(ctx, kit.HeightAtLeast(20))
	for _, m := range miner {
		m.Pause()
	}

	history, err := client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "0x10"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(6, len(history.BaseFeePerGas))
	require.Equal(5, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(16-5+1), history.OldestBlock)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"5", "0x10"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(6, len(history.BaseFeePerGas))
	require.Equal(5, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(16-5+1), history.OldestBlock)
	require.Nil(history.Reward)

	latestBlk, err := client.EthGetBlockByNumber(ctx, "latest", false)
	require.NoError(err)
	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "latest"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(6, len(history.BaseFeePerGas))
	require.Equal(5, len(history.GasUsedRatio))
	require.Equal(latestBlk.Number-5+1, history.OldestBlock)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"0x10", "0x12"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(17, len(history.BaseFeePerGas))
	require.Equal(16, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(18-16+1), history.OldestBlock)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "0x10"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(6, len(history.BaseFeePerGas))
	require.Equal(5, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(16-5+1), history.OldestBlock)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "10"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(6, len(history.BaseFeePerGas))
	require.Equal(5, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(10-5+1), history.OldestBlock)
	require.Nil(history.Reward)

	// test when the requested number of blocks is longer than chain length
	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"0x30", "latest"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(int(latestBlk.Number)+1, len(history.BaseFeePerGas))
	require.Equal(int(latestBlk.Number), len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(1), history.OldestBlock)
	require.Nil(history.Reward)

	// test when the requested number of blocks is longer than chain length
	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{"0x30", "10"}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(10+1, len(history.BaseFeePerGas))
	require.Equal(10, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(1), history.OldestBlock)
	require.Nil(history.Reward)

	history, err = client.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
		json.Marshal([]interface{}{5, "10", &[]float64{25, 50, 75}}),
	).Assert(require.NoError))
	require.NoError(err)
	require.Equal(6, len(history.BaseFeePerGas))
	require.Equal(5, len(history.GasUsedRatio))
	require.Equal(ethtypes.EthUint64(10-5+1), history.OldestBlock)
	require.NotNil(history.Reward)
	require.Equal(5, len(*history.Reward))
	for _, arr := range *history.Reward {
		require.Equal(3, len(arr))
		for _, item := range arr {
			require.Equal(ethtypes.EthBigInt(types.NewInt(full.MinGasPremium)), item)
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
