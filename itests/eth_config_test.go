// stm: #integration
package itests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestEthFilterAPIDisabledViaConfig(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	// pass kit.DisableEthRPC() so RealTimeFilterAPI will not be enabled
	client, _, _ := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())

	_, err := client.EthNewPendingTransactionFilter(ctx)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthGetLogs(ctx, &ethtypes.EthFilterSpec{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthGetFilterChanges(ctx, ethtypes.EthFilterID{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthGetFilterLogs(ctx, ethtypes.EthFilterID{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthNewFilter(ctx, &ethtypes.EthFilterSpec{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthNewBlockFilter(ctx)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthNewPendingTransactionFilter(ctx)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthUninstallFilter(ctx, ethtypes.EthFilterID{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthSubscribe(ctx, []byte{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthUnsubscribe(ctx, ethtypes.EthSubscriptionID{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())
}
