package api

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/big"
)

// Eth tooling matches on these codes, so they must stay those used by reth.
func TestEthErrorWireCodes(t *testing.T) {
	for _, tc := range []struct {
		err  jsonrpc.RPCErrorCodec
		code jsonrpc.ErrorCode
	}{
		{NewErrGasCapExceeded(1000), -32003},
		{NewErrInsufficientFunds(big.NewInt(1), big.NewInt(2)), -32003},
		{NewErrGasAllowance(1000), -32000},
		{NewErrConflictingGasPrices(), -32602},
		{NewErrExpensiveFork(10), -32002},
		{NewErrHeightIndexCold(10, 20), -32002},
	} {
		jerr, err := tc.err.ToJSONRPCError()
		require.NoError(t, err)
		require.Equal(t, tc.code, jerr.Code, jerr.Message)
	}
}

func TestErrResourceUnavailableRoundTrip(t *testing.T) {
	decode := func(t *testing.T, raw []byte) *ErrResourceUnavailable {
		t.Helper()
		var wire jsonrpc.JSONRPCError
		require.NoError(t, json.Unmarshal(raw, &wire))
		var out ErrResourceUnavailable
		require.NoError(t, out.FromJSONRPCError(wire))
		return &out
	}

	for _, tc := range []struct {
		name     string
		in       *ErrResourceUnavailable
		wantJSON string
	}{
		{
			"expensive fork",
			NewErrExpensiveFork(10),
			`{"code":-32002,"message":"required historical state unavailable: refusing explicit call due to state fork at epoch 10","data":{"epoch":10,"reason":"expensive-fork"}}`,
		},
		{
			"cold index with progress",
			NewErrHeightIndexCold(10, 20),
			`{"code":-32002,"message":"timed out resolving epoch 10: node is still indexing chain heights (ready down to epoch 20), retry later","data":{"epoch":10,"reason":"height-index-cold"}}`,
		},
		{
			"cold index without progress",
			NewErrHeightIndexCold(10, -1),
			`{"code":-32002,"message":"timed out resolving epoch 10: node is still indexing chain heights (not started), retry later","data":{"epoch":10,"reason":"height-index-cold"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jerr, err := tc.in.ToJSONRPCError()
			require.NoError(t, err)
			raw, err := json.Marshal(jerr)
			require.NoError(t, err)
			require.JSONEq(t, tc.wantJSON, string(raw))
			require.Equal(t, tc.in, decode(t, raw))
		})
	}

	legacy := decode(t, []byte(`{"code":-32002,"message":"fork","data":7}`))
	require.Equal(t, &ErrResourceUnavailable{Reason: ReasonExpensiveFork, Epoch: 7, Message: "fork"}, legacy)

	legacyCold := decode(t, []byte(`{"code":-32002,"message":"index cold","data":7,"meta":{"reason":"height-index-cold"}}`))
	require.Equal(t, &ErrResourceUnavailable{Reason: ReasonHeightIndexCold, Epoch: 7, Message: "index cold"}, legacyCold)

	for _, raw := range []string{
		`{"code":-32002,"message":"unavailable"}`,
		`{"code":-32002,"message":"unavailable","data":{"reason":"expensive-fork"}}`,
		`{"code":-32002,"message":"unavailable","data":{"epoch":"7","reason":"expensive-fork"}}`,
	} {
		require.Equal(t, &ErrResourceUnavailable{Epoch: -1, Message: "unavailable"}, decode(t, []byte(raw)), raw)
	}
}

func TestErrResourceUnavailableIs(t *testing.T) {
	fork := NewErrExpensiveFork(10)
	cold := NewErrHeightIndexCold(10, 20)
	anyReason := &ErrResourceUnavailable{}
	forkReason := &ErrResourceUnavailable{Reason: ReasonExpensiveFork}

	require.ErrorIs(t, fork, anyReason)
	require.ErrorIs(t, cold, anyReason)
	require.ErrorIs(t, fork, forkReason)
	require.False(t, errors.Is(cold, forkReason))
}
