package itests

import (
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/itests/kit"
	bminer "github.com/filecoin-project/lotus/miner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSDRUpgrade(t *testing.T) {
	kit.QuietMiningLogs()

	// oldDelay := policy.GetPreCommitChallengeDelay()
	// policy.SetPreCommitChallengeDelay(5)
	// t.Cleanup(func() {
	// 	policy.SetPreCommitChallengeDelay(oldDelay)
	// })

	blocktime := 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.SDRUpgradeAt(500, 1000),
	)
	ens.InterconnectAll()

	build.Clock.Sleep(time.Second)

	pledge := make(chan struct{})
	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		round := 0
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := miner.MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}

			// 3 sealing rounds: before, during after.
			if round >= 3 {
				continue
			}

			head, err := client.ChainHead(ctx)
			assert.NoError(t, err)

			// rounds happen every 100 blocks, with a 50 block offset.
			if head.Height() >= abi.ChainEpoch(round*500+50) {
				round++
				pledge <- struct{}{}

				ver, err := client.StateNetworkVersion(ctx, head.Key())
				assert.NoError(t, err)
				switch round {
				case 1:
					assert.Equal(t, network.Version6, ver)
				case 2:
					assert.Equal(t, network.Version7, ver)
				case 3:
					assert.Equal(t, network.Version8, ver)
				}
			}

		}
	}()

	// before.
	miner.PledgeSectors(ctx, 9, 0, pledge)

	s, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})

	for i, id := range s {
		info, err := miner.SectorsStatus(ctx, id, true)
		require.NoError(t, err)
		expectProof := abi.RegisteredSealProof_StackedDrg2KiBV1
		if i >= 3 {
			// after
			expectProof = abi.RegisteredSealProof_StackedDrg2KiBV1_1
		}
		assert.Equal(t, expectProof, info.SealProof, "sector %d, id %d", i, id)
	}

	atomic.StoreInt64(&mine, 0)
	<-done
}
