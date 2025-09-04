package eth

import (
	"context"
	"testing"

	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// mockTipSetResolver is a mock implementation for testing error handling
type mockTipSetResolver struct {
	err error
}

func (m *mockTipSetResolver) GetTipSetByHash(ctx context.Context, blkParam ethtypes.EthHash) (*types.TipSet, error) {
	return nil, m.err
}

func (m *mockTipSetResolver) GetTipsetByBlockNumber(ctx context.Context, blkParam string, strict bool) (*types.TipSet, error) {
	return nil, m.err
}

func (m *mockTipSetResolver) GetTipsetByBlockNumberOrHash(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) (*types.TipSet, error) {
	return nil, m.err
}

// TestEthFeeHistoryErrorHandling tests that EthFeeHistory follows go-ethereum patterns:
// - "not found" errors (ErrNullRound, ipld.IsNotFound) should return (nil, nil)
// - Other errors should return (nil, err)
func TestEthFeeHistoryErrorHandling(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		expectNil   bool
		expectError bool
	}{
		{
			name:        "ErrNullRound should return (nil, nil)",
			err:         &api.ErrNullRound{Epoch: 100},
			expectNil:   true,
			expectError: false,
		},
		{
			name:        "ipld.IsNotFound should return (nil, nil)",
			err:         ipld.ErrNotFound{},
			expectNil:   true,
			expectError: false,
		},
		{
			name:        "Other errors should return (nil, err)",
			err:         xerrors.New("some other error"),
			expectNil:   true,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ethGas := &ethGas{
				tipsetResolver: &mockTipSetResolver{err: tc.err},
			}

			rawParams := []byte(`[1,"0x64",[]]`) // block 100, no reward percentiles
			result, err := ethGas.EthFeeHistory(context.Background(), jsonrpc.RawParams(rawParams))

			if tc.expectNil {
				require.Nil(t, result, "result should be nil according to go-ethereum patterns")
			}

			if tc.expectError {
				require.Error(t, err, "should return error for non-not-found errors")
			} else {
				require.NoError(t, err, "should not return error for not-found errors")
			}
		})
	}
}

// TestEthEstimateGasErrorHandling tests that EthEstimateGas follows go-ethereum patterns:
// - "not found" errors (ErrNullRound, ipld.IsNotFound) should return (nil, nil)
// - Other errors should return (nil, err)
func TestEthEstimateGasErrorHandling(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		expectNil   bool
		expectError bool
	}{
		{
			name:        "ErrNullRound should return (nil, nil)",
			err:         &api.ErrNullRound{Epoch: 100},
			expectNil:   true,
			expectError: false,
		},
		{
			name:        "ipld.IsNotFound should return (nil, nil)",
			err:         ipld.ErrNotFound{},
			expectNil:   true,
			expectError: false,
		},
		{
			name:        "Other errors should return (nil, err)",
			err:         xerrors.New("some other error"),
			expectNil:   true,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ethGas := &ethGas{
				tipsetResolver: &mockTipSetResolver{err: tc.err},
			}

			// Create test parameters that trigger the tipset resolver
			blkParam := ethtypes.NewEthBlockNumberOrHashFromNumber(100)
			params := ethtypes.EthEstimateGasParams{
				Tx: ethtypes.EthCall{
					From: &ethtypes.EthAddress{},
					To:   &ethtypes.EthAddress{},
				},
				BlkParam: &blkParam,
			}

			rawParams, err := params.MarshalJSON()
			require.NoError(t, err)

			result, err := ethGas.EthEstimateGas(context.Background(), jsonrpc.RawParams(rawParams))

			if tc.expectNil {
				require.Nil(t, result, "result should be nil according to go-ethereum patterns")
			}

			if tc.expectError {
				require.Error(t, err, "should return error for non-not-found errors")
			} else {
				require.NoError(t, err, "should not return error for not-found errors")
			}
		})
	}
}
