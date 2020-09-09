package full

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestMedian(t *testing.T) {
	require.Equal(t, types.NewInt(5), medianGasPremium([]gasMeta{
		{big.NewInt(5), build.BlockGasTarget},
	}, 1))

	require.Equal(t, types.NewInt(10), medianGasPremium([]gasMeta{
		{big.NewInt(5), build.BlockGasTarget},
		{big.NewInt(10), build.BlockGasTarget},
	}, 1))

	require.Equal(t, types.NewInt(15), medianGasPremium([]gasMeta{
		{big.NewInt(10), build.BlockGasTarget / 2},
		{big.NewInt(20), build.BlockGasTarget / 2},
	}, 1))

	require.Equal(t, types.NewInt(25), medianGasPremium([]gasMeta{
		{big.NewInt(10), build.BlockGasTarget / 2},
		{big.NewInt(20), build.BlockGasTarget / 2},
		{big.NewInt(30), build.BlockGasTarget / 2},
	}, 1))

	require.Equal(t, types.NewInt(15), medianGasPremium([]gasMeta{
		{big.NewInt(10), build.BlockGasTarget / 2},
		{big.NewInt(20), build.BlockGasTarget / 2},
		{big.NewInt(30), build.BlockGasTarget / 2},
	}, 2))
}
