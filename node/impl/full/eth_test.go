package full

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestEthLogFromEvent(t *testing.T) {
	// basic empty
	data, topics, ok := ethLogFromEvent(nil)
	require.True(t, ok)
	require.Nil(t, data)
	require.Nil(t, topics)

	// basic topic
	data, topics, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.True(t, ok)
	require.Nil(t, data)
	require.Len(t, topics, 1)
	require.Equal(t, topics[0], ethtypes.EthHash{})

	// basic topic with data
	data, topics, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "d",
		Codec: cid.Raw,
		Value: []byte{0x0},
	}})
	require.True(t, ok)
	require.Equal(t, data, []byte{0x0})
	require.Len(t, topics, 1)
	require.Equal(t, topics[0], ethtypes.EthHash{})

	// skip topic
	_, _, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t2",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.False(t, ok)

	// duplicate topic
	_, _, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.False(t, ok)

	// duplicate data
	_, _, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "d",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "d",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.False(t, ok)

	// unknown key is fine
	data, topics, ok = ethLogFromEvent([]types.EventEntry{{
		Flags: 0,
		Key:   "t5",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}, {
		Flags: 0,
		Key:   "t1",
		Codec: cid.Raw,
		Value: make([]byte, 32),
	}})
	require.True(t, ok)
	require.Nil(t, data)
	require.Len(t, topics, 1)
	require.Equal(t, topics[0], ethtypes.EthHash{})
}

func TestReward(t *testing.T) {
	baseFee := big.NewInt(100)
	testcases := []struct {
		maxFeePerGas, maxPriorityFeePerGas big.Int
		answer                             big.Int
	}{
		{maxFeePerGas: big.NewInt(600), maxPriorityFeePerGas: big.NewInt(200), answer: big.NewInt(200)},
		{maxFeePerGas: big.NewInt(600), maxPriorityFeePerGas: big.NewInt(300), answer: big.NewInt(300)},
		{maxFeePerGas: big.NewInt(600), maxPriorityFeePerGas: big.NewInt(500), answer: big.NewInt(500)},
		{maxFeePerGas: big.NewInt(600), maxPriorityFeePerGas: big.NewInt(600), answer: big.NewInt(500)},
		{maxFeePerGas: big.NewInt(600), maxPriorityFeePerGas: big.NewInt(1000), answer: big.NewInt(500)},
		{maxFeePerGas: big.NewInt(50), maxPriorityFeePerGas: big.NewInt(200), answer: big.NewInt(-50)},
	}
	for _, tc := range testcases {
		tx := ethtypes.EthTx{
			MaxFeePerGas:         ethtypes.EthBigInt(tc.maxFeePerGas),
			MaxPriorityFeePerGas: ethtypes.EthBigInt(tc.maxPriorityFeePerGas),
		}
		reward := tx.Reward(baseFee)
		require.Equal(t, 0, reward.Int.Cmp(tc.answer.Int), reward, tc.answer)
	}
}

func TestRewardPercentiles(t *testing.T) {
	testcases := []struct {
		percentiles  []float64
		txGasRewards gasRewardSorter
		answer       []int64
	}{
		{
			percentiles:  []float64{25, 50, 75},
			txGasRewards: []gasRewardTuple{},
			answer:       []int64{MinGasPremium, MinGasPremium, MinGasPremium},
		},
		{
			percentiles: []float64{25, 50, 75, 100},
			txGasRewards: []gasRewardTuple{
				{gas: uint64(0), reward: ethtypes.EthBigInt(big.NewInt(300))},
				{gas: uint64(100), reward: ethtypes.EthBigInt(big.NewInt(200))},
				{gas: uint64(350), reward: ethtypes.EthBigInt(big.NewInt(100))},
				{gas: uint64(500), reward: ethtypes.EthBigInt(big.NewInt(600))},
				{gas: uint64(300), reward: ethtypes.EthBigInt(big.NewInt(700))},
			},
			answer: []int64{200, 700, 700, 700},
		},
	}
	for _, tc := range testcases {
		rewards, totalGasUsed := calculateRewardsAndGasUsed(tc.percentiles, tc.txGasRewards)
		gasUsed := uint64(0)
		for _, tx := range tc.txGasRewards {
			gasUsed += tx.gas
		}
		ans := []ethtypes.EthBigInt{}
		for _, bi := range tc.answer {
			ans = append(ans, ethtypes.EthBigInt(big.NewInt(bi)))
		}
		require.Equal(t, totalGasUsed, gasUsed)
		require.Equal(t, len(ans), len(tc.percentiles))
		require.Equal(t, ans, rewards)
	}
}
