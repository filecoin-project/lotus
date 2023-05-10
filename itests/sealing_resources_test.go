package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/itests/kit"
)

// Regression check for a fix introduced in https://github.com/filecoin-project/lotus/pull/10633
func TestPledgeMaxConcurrentGet(t *testing.T) {
	require.NoError(t, os.Setenv("GET_2K_MAX_CONCURRENT", "1"))
	t.Cleanup(func() {
		require.NoError(t, os.Unsetenv("GET_2K_MAX_CONCURRENT"))
	})

	kit.QuietMiningLogs()

	blockTime := 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, miner, ens := kit.EnsembleMinimal(t) // no mock proofs
	ens.InterconnectAll().BeginMining(blockTime)

	miner.PledgeSectors(ctx, 1, 0, nil)
}
