// stm: #integration
package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestEthFilterAPIDisabledViaConfig(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	// don't pass kit.RealTimeFilterAPI() so ActorEvent.EnableRealTimeFilterAPI is false
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

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

	_, err = client.EthSubscribe(ctx, "newHeads", nil)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())

	_, err = client.EthUnsubscribe(ctx, ethtypes.EthSubscriptionID{})
	require.NotNil(t, err)
	require.Equal(t, err.Error(), api.ErrNotSupported.Error())
}
