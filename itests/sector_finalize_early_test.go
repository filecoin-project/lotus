//stm: #integration
package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestDealsWithFinalizeEarly(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @STORAGE_INFO_001
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	var blockTime = 50 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.ConstructorOpts(
		node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
			return func() (sealiface.Config, error) {
				cf := config.DefaultStorageMiner()
				cf.Sealing.FinalizeEarly = true
				return modules.ToSealingConfig(cf), nil
			}, nil
		})))) // no mock proofs.
	ens.InterconnectAll().BeginMining(blockTime)
	dh := kit.NewDealHarness(t, client, miner, miner)

	ctx := context.Background()

	miner.AddStorage(ctx, t, 1000000000, true, false)
	miner.AddStorage(ctx, t, 1000000000, false, true)

	//stm: @STORAGE_LIST_001
	sl, err := miner.StorageList(ctx)
	require.NoError(t, err)
	for si, d := range sl {
		i, err := miner.StorageInfo(ctx, si)
		require.NoError(t, err)

		fmt.Printf("stor d:%d %+v\n", len(d), i)
	}

	t.Run("single", func(t *testing.T) {
		dh.RunConcurrentDeals(kit.RunConcurrentDealsOpts{N: 1})
	})

	//stm: @STORAGE_LIST_001
	sl, err = miner.StorageList(ctx)
	require.NoError(t, err)
	for si, d := range sl {
		i, err := miner.StorageInfo(ctx, si)
		require.NoError(t, err)

		fmt.Printf("stor d:%d %+v\n", len(d), i)
	}
}
