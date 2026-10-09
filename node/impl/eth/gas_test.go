package eth

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestGasCapError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exitCode exitcode.ExitCode
		gasUsed  int64
		want     error
	}{
		{"BelowInclusionCost", exitcode.SysErrOutOfGas, 0, &api.ErrInvalidInput{}},
		{"PreflightRejection", exitcode.SysErrInsufficientFunds, 0, &api.ErrExecutionReverted{}},
		{"OutOfGasWhileExecuting", exitcode.SysErrOutOfGas, 1, &api.ErrTransactionRejected{}},
		{"RevertWhileExecuting", 33, 1, &api.ErrTransactionRejected{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := &api.InvocResult{MsgRct: &types.MessageReceipt{ExitCode: tc.exitCode, GasUsed: tc.gasUsed}}
			require.IsType(t, tc.want, gasCapError(res, 1000))
		})
	}
	res := &api.InvocResult{MsgRct: &types.MessageReceipt{ExitCode: exitcode.SysErrOutOfGas}}
	require.EqualError(t, gasCapError(res, 1000), "gas required exceeds allowance (1000)")
	res.MsgRct.GasUsed = 1000
	require.EqualError(t, gasCapError(res, 1000), "out of gas: gas required exceeds: 1000")
}

func TestEstimateGasError(t *testing.T) {
	outOfGas := xerrors.Errorf("estimating: %w", &api.ErrOutOfGas{})
	reverted := xerrors.Errorf("estimating: %w", &api.ErrExecutionReverted{Message: "reverted"})
	other := xerrors.New("boom")

	// Registered errors must be returned unwrapped: the JSON-RPC layer matches the exact type.
	require.EqualError(t, estimateGasError(outOfGas, 0),
		fmt.Sprintf("out of gas: gas required exceeds: %d", buildconstants.BlockGasLimit))
	require.IsType(t, &api.ErrTransactionRejected{}, estimateGasError(outOfGas, 0))
	require.EqualError(t, estimateGasError(outOfGas, 1000), "out of gas: gas required exceeds: 1000")
	require.IsType(t, &api.ErrExecutionReverted{}, estimateGasError(reverted, 0))
	require.IsType(t, &api.ErrExecutionReverted{}, estimateGasError(reverted, 1000))
	require.ErrorIs(t, estimateGasError(other, 1000), other)
}

func TestGasAllowance(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		balance, value, price big.Int
		want                  int64
		wantErr               string
	}{
		{"Affordable", big.NewInt(1_000_000), big.Zero(), big.NewInt(10), 100_000, ""},
		{"AfterValue", big.NewInt(1_000_000), big.NewInt(500_000), big.NewInt(10), 50_000, ""},
		{"NilValue", big.NewInt(1_000_000), big.Int{}, big.NewInt(10), 100_000, ""},
		{"AllSpentOnValue", big.NewInt(1_000_000), big.NewInt(1_000_000), big.NewInt(10), 0, ""},
		{"AboveBlockLimit", big.Mul(big.NewInt(1e18), big.NewInt(1e18)), big.Zero(), big.NewInt(1), buildconstants.BlockGasLimit, ""},
		{"ValueExceedsBalance", big.NewInt(1_000_000), big.NewInt(1_000_001), big.NewInt(10), 0,
			"insufficient funds for gas * price + value: have 1000000 want 1000001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gasAllowance(tc.balance, tc.value, tc.price)
			if tc.wantErr != "" {
				require.IsType(t, &api.ErrTransactionRejected{}, err)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGasPrice(t *testing.T) {
	maxFee := ethtypes.EthBigInt(big.NewInt(7))
	unset := gasPrice(ethtypes.EthCall{})
	require.True(t, unset.NilOrZero())
	require.Equal(t, big.NewInt(5), gasPrice(ethtypes.EthCall{GasPrice: ethtypes.EthBigInt(big.NewInt(5))}))
	require.Equal(t, big.NewInt(7), gasPrice(ethtypes.EthCall{GasPrice: ethtypes.EthBigInt(big.NewInt(5)), MaxFeePerGas: &maxFee}))
}

// limitStateManager succeeds a call only when succeeds reports true for its gas limit.
type limitStateManager struct {
	StateManager
	succeeds func(limit int64) bool
}

func (sm *limitStateManager) CallWithGas(_ context.Context, msg *types.Message, _ []types.ChainMsg, _ *types.TipSet, _ bool) (*api.InvocResult, error) {
	exitCode := exitcode.Ok
	if !sm.succeeds(msg.GasLimit) {
		exitCode = exitcode.SysErrOutOfGas
	}
	return &api.InvocResult{MsgRct: &types.MessageReceipt{ExitCode: exitCode, GasUsed: msg.GasLimit}}, nil
}

func TestGasSearchMargin(t *testing.T) {
	search := func(succeeds func(int64) bool, start, gasCap int64) (int64, error) {
		sm := &limitStateManager{succeeds: succeeds}
		return gasSearch(context.Background(), sm, &types.Message{GasLimit: start}, nil, nil, false, gasCap, 1.25)
	}
	atLeast := func(limit int64) bool { return limit >= 1000 }

	gas, err := search(atLeast, 100, 1_000_000)
	require.NoError(t, err)
	require.True(t, atLeast(gas))
	require.Greater(t, gas, int64(1200), "the margin is added")

	gas, err = search(atLeast, 100, 1100)
	require.NoError(t, err)
	require.Equal(t, int64(1100), gas, "the margin is clamped to the cap")

	// Fails with the margin, as a contract that checks gasleft() can.
	notInBand := func(limit int64) bool { return atLeast(limit) && (limit < 1100 || limit > 1500) }
	gas, err = search(notInBand, 1000, 1_000_000)
	require.NoError(t, err)
	require.Equal(t, int64(1000), gas, "the tested limit is kept")

	_, err = search(atLeast, 100, 900)
	require.IsType(t, &api.ErrTransactionRejected{}, err)
}
