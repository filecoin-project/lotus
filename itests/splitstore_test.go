//stm: #integration
package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/blockstore/splitstore"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

// Startup a node with hotstore and discard coldstore.  Compact once and return
func TestHotstoreCompactsOnce(t *testing.T) {

	ctx := context.Background()
	// disable sync checking because efficient itests require that the node is out of sync : /
	splitstore.CheckSyncGap = false
	opts := []interface{}{kit.MockProofs(), kit.WithCfgOpt(kit.SplitstoreDiscard())}
	full, genesisMiner, ens := kit.EnsembleMinimal(t, opts...)
	fmt.Printf("begin mining\n")
	ens.InterconnectAll().BeginMining(4 * time.Millisecond)
	_ = full
	_ = genesisMiner
	for {
		time.Sleep(1 * time.Second)
		info, err := full.ChainBlockstoreInfo(ctx)
		require.NoError(t, err)
		compact, ok := info["compactions"]
		require.True(t, ok, "compactions not on blockstore info")
		compactionIndex, ok := compact.(int64)
		require.True(t, ok, "compaction key on blockstore info wrong type")
		fmt.Printf("compactions: %d\n", compactionIndex)
		if compactionIndex >= 1 {
			break
		}
	}
	require.NoError(t, genesisMiner.Stop(ctx))
}

//, create some unreachable state
// and check that compaction carries it away
