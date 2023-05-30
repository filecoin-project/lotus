// stm: #integration
package itests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

func TestMinerBalanceCollateral(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @MINER_SECTOR_LIST_001
	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond

	runTest := func(t *testing.T, enabled bool, nSectors int, batching bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		opts := kit.ConstructorOpts(
			node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
				return func() (sealiface.Config, error) {
					cfg := config.DefaultStorageMiner()
					sc := modules.ToSealingConfig(cfg.Dealmaking, cfg.Sealing)

					sc.MaxWaitDealsSectors = 4
					sc.MaxSealingSectors = 4
					sc.MaxSealingSectorsForDeals = 4
					sc.AlwaysKeepUnsealedCopy = true
					sc.WaitDealsDelay = time.Hour

					sc.BatchPreCommits = batching
					sc.AggregateCommits = batching

					sc.PreCommitBatchWait = time.Hour
					sc.CommitBatchWait = time.Hour

					sc.MinCommitBatch = nSectors
					sc.MaxPreCommitBatch = nSectors
					sc.MaxCommitBatch = nSectors

					sc.CollateralFromMinerBalance = enabled
					sc.AvailableBalanceBuffer = big.Zero()
					sc.DisableCollateralFallback = false
					sc.AggregateAboveBaseFee = big.Zero()
					sc.BatchPreCommitAboveBaseFee = big.Zero()

					return sc, nil
				}, nil
			})),
		)
		full, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), opts)

		ens.InterconnectAll().BeginMining(blockTime)
		full.WaitTillChain(ctx, kit.HeightAtLeast(10))

		toCheck := miner.StartPledge(ctx, nSectors, 0, nil)

		for len(toCheck) > 0 {
			states := map[api.SectorState]int{}
			for n := range toCheck {
				st, err := miner.StorageMiner.SectorsStatus(ctx, n, false)
				require.NoError(t, err)
				states[st.State]++
				if st.State == api.SectorState(sealing.Proving) {
					delete(toCheck, n)
				}
				if strings.Contains(string(st.State), "Fail") {
					t.Fatal("sector in a failed state", st.State)
				}
			}

			build.Clock.Sleep(100 * time.Millisecond)
		}

		// check that sector messages had zero value set
		sl, err := miner.SectorsListNonGenesis(ctx)
		require.NoError(t, err)

		for _, number := range sl {
			si, err := miner.SectorsStatus(ctx, number, false)
			require.NoError(t, err)

			require.NotNil(t, si.PreCommitMsg)
			pc, err := full.ChainGetMessage(ctx, *si.PreCommitMsg)
			require.NoError(t, err)
			if enabled {
				require.Equal(t, big.Zero(), pc.Value)
			} else {
				require.NotEqual(t, big.Zero(), pc.Value)
			}

			require.NotNil(t, si.CommitMsg)
			c, err := full.ChainGetMessage(ctx, *si.CommitMsg)
			require.NoError(t, err)
			if enabled {
				require.Equal(t, big.Zero(), c.Value)
			}
			// commit value might be zero even with !enabled because in test devnets
			//  precommit deposit tends to be greater than collateral required at
			//  commit time.
		}
	}

	t.Run("nobatch", func(t *testing.T) {
		runTest(t, true, 1, false)
	})
	t.Run("batch-1", func(t *testing.T) {
		runTest(t, true, 1, true) // individual commit instead of aggregate
	})
	t.Run("batch-4", func(t *testing.T) {
		runTest(t, true, 4, true)
	})

	t.Run("nobatch-frombalance-disabled", func(t *testing.T) {
		runTest(t, false, 1, false)
	})
	t.Run("batch-1-frombalance-disabled", func(t *testing.T) {
		runTest(t, false, 1, true) // individual commit instead of aggregate
	})
	t.Run("batch-4-frombalance-disabled", func(t *testing.T) {
		runTest(t, false, 4, true)
	})
}
