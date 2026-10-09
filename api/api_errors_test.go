package api

import (
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
		{NewErrNegativeGasPrice(), -32602},
	} {
		jerr, err := tc.err.ToJSONRPCError()
		require.NoError(t, err)
		require.Equal(t, tc.code, jerr.Code, jerr.Message)
	}
}
