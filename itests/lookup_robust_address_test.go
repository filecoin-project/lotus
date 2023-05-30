package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestStateLookupRobustAddress(t *testing.T) {
	ctx := context.Background()
	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version15))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	addr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	// Look up the robust address
	robAddr, err := client.StateLookupRobustAddress(ctx, addr, types.EmptyTSK)
	require.NoError(t, err)

	// Check the id address for the given robust address and make sure it matches
	idAddr, err := client.StateLookupID(ctx, robAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, addr, idAddr)
}
