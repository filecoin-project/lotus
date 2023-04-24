package retrievalmarket_test

import (
	"bytes"
	"encoding/json"
	"testing"

	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	"github.com/libp2p/go-libp2p/core/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestParamsMarshalUnmarshal(t *testing.T) {
	pieceCid := tut.GenerateCids(1)[0]

	allSelector := selectorparse.CommonSelector_ExploreAllRecursively
	params, err := retrievalmarket.NewParamsV1(abi.NewTokenAmount(123), 456, 789, allSelector, &pieceCid, big.Zero())
	assert.NoError(t, err)

	buf := new(bytes.Buffer)
	err = params.MarshalCBOR(buf)
	assert.NoError(t, err)

	unmarshalled := &retrievalmarket.Params{}
	err = unmarshalled.UnmarshalCBOR(buf)
	assert.NoError(t, err)

	assert.Equal(t, params, *unmarshalled)

	assert.Equal(t, unmarshalled.Selector.Node, allSelector)
}

func TestPricingInputMarshalUnmarshalJSON(t *testing.T) {
	pid := test.RandPeerIDFatal(t)

	in := retrievalmarket.PricingInput{
		PayloadCID:   tut.GenerateCids(1)[0],
		PieceCID:     tut.GenerateCids(1)[0],
		PieceSize:    abi.UnpaddedPieceSize(100),
		Client:       pid,
		VerifiedDeal: true,
		Unsealed:     true,
		CurrentAsk: retrievalmarket.Ask{
			PricePerByte:            big.Zero(),
			UnsealPrice:             big.Zero(),
			PaymentInterval:         0,
			PaymentIntervalIncrease: 0,
		},
	}

	bz, err := json.Marshal(in)
	require.NoError(t, err)

	resp2 := retrievalmarket.PricingInput{}
	require.NoError(t, json.Unmarshal(bz, &resp2))

	require.Equal(t, in, resp2)
}

func TestParamsIntervalBounds(t *testing.T) {
	testCases := []struct {
		name             string
		currentInterval  uint64
		paymentInterval  uint64
		intervalIncrease uint64
		expLowerBound    uint64
		expNextInterval  uint64
	}{{
		currentInterval:  0,
		paymentInterval:  10,
		intervalIncrease: 5,
		expLowerBound:    0,
		expNextInterval:  10,
	}, {
		currentInterval:  10,
		paymentInterval:  10,
		intervalIncrease: 5,
		expLowerBound:    10,
		expNextInterval:  25, // 10 + (10 + 5)
	}, {
		currentInterval:  25,
		paymentInterval:  10,
		intervalIncrease: 5,
		expLowerBound:    25,
		expNextInterval:  45, // 10 + (10 + 5) + (10 + 5 + 5)
	}, {
		currentInterval:  45,
		paymentInterval:  10,
		intervalIncrease: 5,
		expLowerBound:    45,
		expNextInterval:  70, // 10 + (10 + 5) + (10 + 5 + 5) + (10 + 5 + 5 + 5)
	}}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			pricePerByte := abi.NewTokenAmount(1)
			params := retrievalmarket.Params{
				PricePerByte:            pricePerByte,
				PaymentInterval:         tc.paymentInterval,
				PaymentIntervalIncrease: tc.intervalIncrease,
			}
			currentIntervalFundPrice := big.Mul(pricePerByte, big.NewIntUnsigned(tc.currentInterval))
			lowerBound := params.IntervalLowerBound(tc.currentInterval)
			nextInterval := params.NextInterval(currentIntervalFundPrice)

			require.Equal(t, tc.expLowerBound, lowerBound)
			require.Equal(t, tc.expNextInterval, nextInterval)
		})
	}
}
