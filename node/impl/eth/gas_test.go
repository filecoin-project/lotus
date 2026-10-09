package eth

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestGasCapError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exitCode exitcode.ExitCode
		gasUsed  int64
		want     error
	}{
		{"BelowInclusionCost", exitcode.SysErrOutOfGas, 0, &api.ErrGasAllowance{}},
		{"PreflightRejection", exitcode.SysErrInsufficientFunds, 0, &api.ErrExecutionReverted{}},
		{"OutOfGasWhileExecuting", exitcode.SysErrOutOfGas, 1, &api.ErrGasCapExceeded{}},
		{"RevertWhileExecuting", 33, 1, &api.ErrGasCapExceeded{}},
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
	require.IsType(t, &api.ErrOutOfGas{}, estimateGasError(outOfGas, 0))
	require.IsType(t, &api.ErrGasCapExceeded{}, estimateGasError(outOfGas, 1000))
	require.IsType(t, &api.ErrExecutionReverted{}, estimateGasError(reverted, 0))
	require.IsType(t, &api.ErrExecutionReverted{}, estimateGasError(reverted, 1000))
	require.ErrorIs(t, estimateGasError(other, 1000), other)
}
