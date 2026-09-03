package miner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/network"
)

func TestValidateUpgradeQualityNetworkVersion(t *testing.T) {
	require.NoError(t, validateUpgradeQualityNetworkVersion(network.Version28))
	require.NoError(t, validateUpgradeQualityNetworkVersion(network.Version29))
	require.ErrorContains(t, validateUpgradeQualityNetworkVersion(network.Version27), "network version 28")
}

func TestLegacyUpgradeQualityCompatMessage(t *testing.T) {
	require.ErrorContains(t, legacyUpgradeQualityCompatMessage(network.Version28), "network version 28")
	require.NoError(t, legacyUpgradeQualityCompatMessage(network.Version29))
}
