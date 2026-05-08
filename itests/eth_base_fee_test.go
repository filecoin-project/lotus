package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestEthBaseFee verifies that eth_baseFee returns the minimum base fee on an
// idle test network. With no transactions, blocks are empty and the base fee
// decays to its floor (buildconstants.MinimumBaseFee) after a few blocks.
func TestEthBaseFee(t *testing.T) {
	kit.QuietAllLogsExcept()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ens.InterconnectAll().BeginMining(blockTime)

	// Empty blocks drive the base fee down to the minimum within a few epochs.
	client.WaitTillChain(ctx, kit.HeightAtLeast(10))

	baseFee, err := client.EthBaseFee(ctx)
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthBigInt(big.NewInt(buildconstants.MinimumBaseFee)), baseFee)
}
