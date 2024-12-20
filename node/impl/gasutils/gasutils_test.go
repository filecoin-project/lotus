package gasutils

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestMedian(t *testing.T) {
	require.Equal(t, types.NewInt(5), medianGasPremium([]GasMeta{
		{big.NewInt(5), buildconstants.BlockGasTarget},
	}, 1))

	require.Equal(t, types.NewInt(10), medianGasPremium([]GasMeta{
		{big.NewInt(5), buildconstants.BlockGasTarget},
		{big.NewInt(10), buildconstants.BlockGasTarget},
	}, 1))

	require.Equal(t, types.NewInt(15), medianGasPremium([]GasMeta{
		{big.NewInt(10), buildconstants.BlockGasTarget / 2},
		{big.NewInt(20), buildconstants.BlockGasTarget / 2},
	}, 1))

	require.Equal(t, types.NewInt(25), medianGasPremium([]GasMeta{
		{big.NewInt(10), buildconstants.BlockGasTarget / 2},
		{big.NewInt(20), buildconstants.BlockGasTarget / 2},
		{big.NewInt(30), buildconstants.BlockGasTarget / 2},
	}, 1))

	require.Equal(t, types.NewInt(15), medianGasPremium([]GasMeta{
		{big.NewInt(10), buildconstants.BlockGasTarget / 2},
		{big.NewInt(20), buildconstants.BlockGasTarget / 2},
		{big.NewInt(30), buildconstants.BlockGasTarget / 2},
	}, 2))
}
