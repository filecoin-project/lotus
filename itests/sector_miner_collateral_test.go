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
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestMinerBalanceCollateral(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 5 * time.Millisecond

	runTest := func(t *testing.T, enabled bool, nSectors int, batching bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		opts := kit.ConstructorOpts(
			node.ApplyIf(node.IsType(repo.StorageMiner), node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
				return func() (sealiface.Config, error) {
					return sealiface.Config{
						MaxWaitDealsSectors:       4,
						MaxSealingSectors:         4,
						MaxSealingSectorsForDeals: 4,
						AlwaysKeepUnsealedCopy:    true,
						WaitDealsDelay:            time.Hour,

						BatchPreCommits:  batching,
						AggregateCommits: batching,

						PreCommitBatchWait: time.Hour,
						CommitBatchWait:    time.Hour,

						MinCommitBatch:    nSectors,
						MaxPreCommitBatch: nSectors,
						MaxCommitBatch:    nSectors,

						CollateralFromMinerBalance: enabled,
						AvailableBalanceBuffer:     big.Zero(),
						DisableCollateralFallback:  false,
						AggregateAboveBaseFee:      big.Zero(),
						BatchPreCommitAboveBaseFee: big.Zero(),
					}, nil
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
		sl, err := miner.SectorsList(ctx)
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
