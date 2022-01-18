package itests

import (
	"context"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/storage"
)

func TestWorkerPledge(t *testing.T) {
	ctx := context.Background()
	_, miner, worker, ens := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithNoLocalSealing(true),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTFetch, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2, sealtasks.TTUnseal})) // no mock proofs

	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	miner.PledgeSectors(ctx, 1, 0, nil)
}

func TestWinningPostWorker(t *testing.T) {
	prevIns := build.InsecurePoStValidation
	build.InsecurePoStValidation = false
	defer func() {
		build.InsecurePoStValidation = prevIns
	}()

	ctx := context.Background()
	client, _, worker, ens := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWinningPoSt})) // no mock proofs

	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	client.WaitTillChain(ctx, kit.HeightAtLeast(6))
}

func TestWindowPostWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	client, miner, _, ens := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}))

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	bm := ens.InterconnectAll().BeginMining(2 * time.Millisecond)[0]

	di = di.NextNotElapsed()

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStChallengeWindow*2 + storage.SubmitConfidence
	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))

	t.Log("Waiting for post message")
	bm.Stop()

	for i := 0; i < 500; i++ {
		n, err := client.MpoolPending(ctx, types.EmptyTSK)
		require.NoError(t, err)

		if len(n) > 0 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}

	n, err := client.MpoolPending(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.Greater(t, len(n), 0)

	t.Log("post message landed")

	bm.MineBlocks(ctx, 2*time.Millisecond)

	waitUntil = di.Open + di.WPoStChallengeWindow*3
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(sectors)))
}
