package itests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

func TestPledgeSectors(t *testing.T) {

	kit.QuietMiningLogs()

	blockTime := 50 * time.Millisecond

	runTest := func(t *testing.T, nSectors int) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
		ens.InterconnectAll().BeginMiningMustPost(blockTime)

		miner.PledgeSectors(ctx, nSectors, 0, nil)
	}

	t.Run("1", func(t *testing.T) {
		runTest(t, 1)
	})

	t.Run("100", func(t *testing.T) {
		runTest(t, 100)
	})

	t.Run("1000", func(t *testing.T) {
		if testing.Short() { // takes ~16s
			t.Skip("skipping test in short mode")
		}

		runTest(t, 1000)
	})
}

func TestPledgeBatching(t *testing.T) {
	blockTime := 50 * time.Millisecond

	runTest := func(t *testing.T, nSectors int, aggregate bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		kit.QuietMiningLogs()

		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(!aggregate))
		ens.InterconnectAll().BeginMiningMustPost(blockTime)

		client.WaitTillChain(ctx, kit.HeightAtLeast(10))

		toCheck := miner.StartPledge(ctx, nSectors, 0, nil)

		for len(toCheck) > 0 {
			states := map[api.SectorState]int{}

			for n := range toCheck {
				st, err := miner.SectorsStatus(ctx, n, false)
				require.NoError(t, err)
				states[st.State]++
				if st.State == api.SectorState(sealing.Proving) {
					delete(toCheck, n)
				}
				if strings.Contains(string(st.State), "Fail") {
					t.Fatal("sector in a failed state", st.State)
				}
			}
			if states[api.SectorState(sealing.SubmitPreCommitBatch)] == nSectors ||
				(states[api.SectorState(sealing.SubmitPreCommitBatch)] > 0 && states[api.SectorState(sealing.PreCommit1)] == 0 && states[api.SectorState(sealing.PreCommit2)] == 0) {
				pcb, err := miner.SectorPreCommitFlush(ctx)
				require.NoError(t, err)
				if pcb != nil {
					fmt.Printf("PRECOMMIT BATCH: %+v\n", pcb)
				}
			}

			if states[api.SectorState(sealing.SubmitCommitAggregate)] == nSectors ||
				(states[api.SectorState(sealing.SubmitCommitAggregate)] > 0 && states[api.SectorState(sealing.WaitSeed)] == 0 && states[api.SectorState(sealing.Committing)] == 0) {
				cb, err := miner.SectorCommitFlush(ctx)
				require.NoError(t, err)
				if cb != nil {
					fmt.Printf("COMMIT BATCH: %+v\n", cb)
				}
			}

			build.Clock.Sleep(100 * time.Millisecond)
			fmt.Printf("WaitSeal: %d %+v\n", len(toCheck), states)
		}
	}

	t.Run("100", func(t *testing.T) {
		runTest(t, 100, false)
	})
	t.Run("10-agg", func(t *testing.T) {
		runTest(t, 10, true)
	})
}

func TestPledgeMaxBatching(t *testing.T) {

	blockTime := 50 * time.Millisecond

	runTest := func(t *testing.T) {
		nSectors := miner5.MaxAggregatedSectors
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, full, miner, ens := kit.EnsembleTwoOne(t, kit.MockProofs())
		ens.InterconnectAll().BeginMining(blockTime)
		m, ok := miner.StorageMiner.(*impl.StorageMinerAPI)
		require.True(t, ok)
		cfg, err := m.GetSealingConfigFunc()
		require.NoError(t, err)
		cfg.MinCommitBatch = miner5.MaxAggregatedSectors
		require.NoError(t, m.SetSealingConfigFunc(cfg))

		toCheck := miner.StartPledge(ctx, nSectors, 0, nil)
		var lastSectorNo abi.SectorNumber

		for len(toCheck) > 0 {
			states := map[api.SectorState]int{}

			for n := range toCheck {
				lastSectorNo = n
				st, err := miner.SectorsStatus(ctx, n, false)
				require.NoError(t, err)
				states[st.State]++
				if st.State == api.SectorState(sealing.Proving) {
					delete(toCheck, n)
				}
				if strings.Contains(string(st.State), "Fail") {
					t.Fatal("sector in a failed state", st.State)
				}
			}
			if states[api.SectorState(sealing.SubmitPreCommitBatch)] == nSectors ||
				(states[api.SectorState(sealing.SubmitPreCommitBatch)] > 0 && states[api.SectorState(sealing.PreCommit1)] == 0 && states[api.SectorState(sealing.PreCommit2)] == 0) {
				pcb, err := miner.SectorPreCommitFlush(ctx)
				require.NoError(t, err)
				if pcb != nil {
					fmt.Printf("PRECOMMIT BATCH: %+v\n", pcb)
				}
			}

			if states[api.SectorState(sealing.SubmitCommitAggregate)] == nSectors {
				cb, err := miner.SectorCommitFlush(ctx)
				require.NoError(t, err)
				if cb != nil {
					fmt.Printf("COMMIT BATCH: %+v\n", cb)
				}
			}

			build.Clock.Sleep(100 * time.Millisecond)
			fmt.Printf("WaitSeal: %d %+v\n", len(toCheck), states)
		}

		// Wait for flushed ProveCommitAggregate to land on chain
		st, err := miner.SectorsStatus(ctx, lastSectorNo, false)
		require.NoError(t, err)
		for st.State == api.SectorState(sealing.CommitAggregateWait) {
			build.Clock.Sleep(100 * time.Millisecond)
		}

		// Ensure that max aggregate message has propagated to the other node by checking current state
		sectorInfosAfter, err := full.StateMinerSectors(ctx, miner.ActorAddr, nil, types.EmptyTSK)
		require.NoError(t, err)
		assert.Equal(t, miner5.MaxAggregatedSectors+kit.DefaultPresealsPerBootstrapMiner, len(sectorInfosAfter))
	}

	t.Run("Force max prove commit aggregate size", runTest)
}

func TestPledgeSynth(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 50 * time.Millisecond

	runTest := func(t *testing.T, nSectors int) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, miner, ens := kit.EnsembleMinimal(t, kit.MutateSealingConfig(func(sc *config.SealingConfig) {
			sc.UseSyntheticPoRep = true
		})) // no mock proofs

		ens.InterconnectAll().BeginMiningMustPost(blockTime)

		miner.PledgeSectors(ctx, nSectors, 0, nil)
	}

	t.Run("1", func(t *testing.T) {
		runTest(t, 1)
	})

	t.Run("3", func(t *testing.T) {
		runTest(t, 3)
	})
}

func TestSectorsSummary(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nPreseal := 2

	_, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.PresealSectors(nPreseal))
	ens.InterconnectAll().BeginMining(blockTime)

	miner.PledgeSectors(ctx, 1, 0, nil)

	ms, err := miner.SectorsSummary(ctx)
	require.NoError(t, err)

	require.Len(t, ms, 1) // all proving

	for st, n := range ms {
		require.Equal(t, api.SectorState(sealing.Proving), st)
		require.Equal(t, 1+nPreseal, n)
	}
}
