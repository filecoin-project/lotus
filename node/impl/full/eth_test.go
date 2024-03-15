package full

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestParseBlockRange(t *testing.T) {
	pstring := func(s string) *string { return &s }

	tcs := map[string]struct {
		heaviest abi.ChainEpoch
		from     *string
		to       *string
		maxRange abi.ChainEpoch
		minOut   abi.ChainEpoch
		maxOut   abi.ChainEpoch
		errStr   string
	}{
		"fails when both are specified and range is greater than max allowed range": {
			heaviest: 100,
			from:     pstring("0x100"),
			to:       pstring("0x200"),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "too large",
		},
		"fails when min is specified and range is greater than max allowed range": {
			heaviest: 500,
			from:     pstring("0x10"),
			to:       pstring("latest"),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "too far in the past",
		},
		"fails when max is specified and range is greater than max allowed range": {
			heaviest: 500,
			from:     pstring("earliest"),
			to:       pstring("0x10000"),
			maxRange: 10,
			minOut:   0,
			maxOut:   0,
			errStr:   "too large",
		},
		"works when range is valid": {
			heaviest: 500,
			from:     pstring("earliest"),
			to:       pstring("latest"),
			maxRange: 1000,
			minOut:   0,
			maxOut:   -1,
		},
		"works when range is valid and specified": {
			heaviest: 500,
			from:     pstring("0x10"),
			to:       pstring("0x30"),
			maxRange: 1000,
			minOut:   16,
			maxOut:   48,
		},
	}

	for name, tc := range tcs {
		tc2 := tc
		t.Run(name, func(t *testing.T) {
			min, max, err := parseBlockRange(tc2.heaviest, tc2.from, tc2.to, tc2.maxRange)
			require.Equal(t, tc2.minOut, min)
			require.Equal(t, tc2.maxOut, max)
			if tc2.errStr != "" {
				fmt.Println(err)
				require.Error(t, err)
				require.Contains(t, err.Error(), tc2.errStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEthLogFromEvent(t *testing.T) {
	// basic empty
	data, topics, ok := ethLogFromEvent(nil)
	require.True(t, ok)
	require.Nil(t, data)
	require.Empty(t, topics)
	require.NotNil(t, topics)

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
		{maxFeePerGas: big.NewInt(50), maxPriorityFeePerGas: big.NewInt(200), answer: big.NewInt(0)},
	}
	for _, tc := range testcases {
		msg := &types.Message{GasFeeCap: tc.maxFeePerGas, GasPremium: tc.maxPriorityFeePerGas}
		reward := msg.EffectiveGasPremium(baseFee)
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
				{gasUsed: int64(0), premium: big.NewInt(300)},
				{gasUsed: int64(100), premium: big.NewInt(200)},
				{gasUsed: int64(350), premium: big.NewInt(100)},
				{gasUsed: int64(500), premium: big.NewInt(600)},
				{gasUsed: int64(300), premium: big.NewInt(700)},
			},
			answer: []int64{200, 700, 700, 700},
		},
	}
	for _, tc := range testcases {
		rewards, totalGasUsed := calculateRewardsAndGasUsed(tc.percentiles, tc.txGasRewards)
		var gasUsed int64
		for _, tx := range tc.txGasRewards {
			gasUsed += tx.gasUsed
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

func TestABIEncoding(t *testing.T) {
	// Generated from https://abi.hashex.org/
	const expected = "000000000000000000000000000000000000000000000000000000000000001600000000000000000000000000000000000000000000000000000000000000510000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000001b1111111111111111111020200301000000044444444444444444010000000000"
	const data = "111111111111111111102020030100000004444444444444444401"

	expectedBytes, err := hex.DecodeString(expected)
	require.NoError(t, err)

	dataBytes, err := hex.DecodeString(data)
	require.NoError(t, err)

	require.Equal(t, expectedBytes, encodeAsABIHelper(22, 81, dataBytes))
}

func TestDecodePayload(t *testing.T) {
	// "empty"
	b, err := decodePayload(nil, 0)
	require.NoError(t, err)
	require.Empty(t, b)

	// raw empty
	_, err = decodePayload(nil, uint64(multicodec.Raw))
	require.NoError(t, err)
	require.Empty(t, b)

	// raw non-empty
	b, err = decodePayload([]byte{1}, uint64(multicodec.Raw))
	require.NoError(t, err)
	require.EqualValues(t, b, []byte{1})

	// Invalid cbor bytes
	_, err = decodePayload(nil, uint64(multicodec.DagCbor))
	require.Error(t, err)

	// valid cbor bytes
	var w bytes.Buffer
	require.NoError(t, cbg.WriteByteArray(&w, []byte{1}))
	b, err = decodePayload(w.Bytes(), uint64(multicodec.DagCbor))
	require.NoError(t, err)
	require.EqualValues(t, b, []byte{1})

	// regular cbor also works.
	b, err = decodePayload(w.Bytes(), uint64(multicodec.Cbor))
	require.NoError(t, err)
	require.EqualValues(t, b, []byte{1})

	// random codec should fail
	_, err = decodePayload(w.Bytes(), 42)
	require.Error(t, err)
}
