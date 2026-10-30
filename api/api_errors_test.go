package api

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
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
		{NewErrNegativeGasPrice(), -32602},
		{NewErrExpensiveFork(10), -32002},
		{NewErrHeightIndexCold(10, 20), -32002},
	} {
		jerr, err := tc.err.ToJSONRPCError()
		require.NoError(t, err)
		require.Equal(t, tc.code, jerr.Code, jerr.Message)
	}
}

func TestErrResourceUnavailableRoundTrip(t *testing.T) {
	roundTrip := func(t *testing.T, jerr jsonrpc.JSONRPCError) *ErrResourceUnavailable {
		// Data crosses the wire as JSON, so numbers decode as float64.
		raw, err := json.Marshal(jerr)
		require.NoError(t, err)
		var wire jsonrpc.JSONRPCError
		require.NoError(t, json.Unmarshal(raw, &wire))
		var out ErrResourceUnavailable
		require.NoError(t, out.FromJSONRPCError(wire))
		return &out
	}

	for _, in := range []*ErrResourceUnavailable{
		NewErrExpensiveFork(10),
		NewErrHeightIndexCold(10, 20),
		NewErrHeightIndexCold(10, -1),
	} {
		jerr, err := in.ToJSONRPCError()
		require.NoError(t, err)
		require.Equal(t, abi.ChainEpoch(10), jerr.Data)
		require.Equal(t, in, roundTrip(t, jerr))
	}

	// Older nodes send no meta and only refused expensive forks.
	legacy := roundTrip(t, jsonrpc.JSONRPCError{Code: EResourceUnavailable, Message: "fork", Data: 7})
	require.Equal(t, &ErrResourceUnavailable{Reason: ReasonExpensiveFork, Epoch: 7, Message: "fork"}, legacy)
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
