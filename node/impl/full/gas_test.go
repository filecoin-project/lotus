package full

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

// gasModuleWithMaxFee returns a GasModule whose GetMaxFee returns the given amount.
// All other fields are nil, which is safe as long as the test does not reach
// any code path that requires chain state (i.e. GasLimit/GasFeeCap/GasPremium
// must all be pre-supplied so the estimation steps are skipped).
func gasModuleWithMaxFee(maxFee abi.TokenAmount) *GasModule {
	return &GasModule{
		GetMaxFee: func() (abi.TokenAmount, error) { return maxFee, nil },
	}
}

func TestGasEstimateMessageGasUndefinedAddresses(t *testing.T) {
	m := gasModuleWithMaxFee(big.NewInt(1e18))
	msg := &types.Message{
		Value:      big.Zero(),
		GasLimit:   1,
		GasFeeCap:  big.NewInt(1),
		GasPremium: big.NewInt(1),
	}

	t.Run("undefined To", func(t *testing.T) {
		msg.To = address.Undef
		msg.From = address.TestAddress
		_, err := m.GasEstimateMessageGas(context.Background(), msg, nil, types.EmptyTSK)
		require.ErrorContains(t, err, "To address is undefined")
	})

	t.Run("undefined From", func(t *testing.T) {
		msg.To = address.TestAddress
		msg.From = address.Undef
		_, err := m.GasEstimateMessageGas(context.Background(), msg, nil, types.EmptyTSK)
		require.ErrorContains(t, err, "From address is undefined")
	})
}

func TestGasEstimateMessageGasSend(t *testing.T) {
	m := gasModuleWithMaxFee(big.NewInt(1e18))
	msg := &types.Message{
		To:         builtin.BurntFundsActorAddr,
		From:       address.TestAddress,
		Value:      big.Zero(),
		GasLimit:   1_000_000,
		GasFeeCap:  big.NewInt(1_000_000),
		GasPremium: big.NewInt(1_000),
	}

	out, err := m.GasEstimateMessageGas(context.Background(), msg, nil, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, builtin.BurntFundsActorAddr, out.To)
	require.Equal(t, address.TestAddress, out.From)
	require.EqualValues(t, 1_000_000, out.GasLimit)
}
