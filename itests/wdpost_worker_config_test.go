package itests

import (
	"context"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

func TestWindowPostNoBuiltinWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	client, miner, _, ens := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ConstructorOpts(
			node.Override(new(config.ProvingConfig), func() config.ProvingConfig {
				c := config.DefaultStorageMiner()
				c.Proving.DisableBuiltinWindowPoSt = true
				return c.Proving
			}),
			node.Override(new(*wdpost.WindowPoStScheduler), modules.WindowPostScheduler(
				config.DefaultStorageMiner().Fees,
				config.ProvingConfig{
					DisableWDPoStPreChecks: false,
				},
			))),
		kit.ThroughRPC()) // generic non-post worker

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	bm := ens.InterconnectAll().BeginMining(2 * time.Millisecond)[0]

	di = di.NextNotElapsed()

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStChallengeWindow*2 + wdpost.SubmitConfidence
	client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))

	t.Log("Waiting for post message")
	bm.Stop()

	var lastPending []*types.SignedMessage
	for i := 0; i < 500; i++ {
		lastPending, err = client.MpoolPending(ctx, types.EmptyTSK)
		require.NoError(t, err)

		if len(lastPending) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// expect no post messages
	require.Equal(t, len(lastPending), 0)
}

func TestWindowPostNoBuiltinWindowWithWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	client, miner, _, ens := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ConstructorOpts(
			node.Override(new(config.ProvingConfig), func() config.ProvingConfig {
				c := config.DefaultStorageMiner()
				c.Proving.DisableBuiltinWindowPoSt = true
				return c.Proving
			}),
			node.Override(new(*wdpost.WindowPoStScheduler), modules.WindowPostScheduler(
				config.DefaultStorageMiner().Fees,
				config.ProvingConfig{
					DisableBuiltinWindowPoSt:  true,
					DisableBuiltinWinningPoSt: false,
					DisableWDPoStPreChecks:    false,
				},
			))),
		kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}))

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	bm := ens.InterconnectAll().BeginMining(2 * time.Millisecond)[0]

	di = di.NextNotElapsed()

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStChallengeWindow*2 + wdpost.SubmitConfidence
	client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))

	t.Log("Waiting for post message")
	bm.Stop()

	var lastPending []*types.SignedMessage
	for i := 0; i < 500; i++ {
		lastPending, err = client.MpoolPending(ctx, types.EmptyTSK)
		require.NoError(t, err)

		if len(lastPending) > 0 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}

	require.Greater(t, len(lastPending), 0)

	t.Log("post message landed")

	bm.MineBlocksMustPost(ctx, 2*time.Millisecond)

	waitUntil = di.Open + di.WPoStChallengeWindow*3
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(sectors)))

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	// Remove one sector in the next deadline (so it's skipped)
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, di.Index+1, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the sector
		sid, err := secs.First()
		require.NoError(t, err)

		t.Logf("Drop sector %d; dl %d part %d", sid, di.Index+1, 0)

		err = miner.BaseAPI.(*impl.StorageMinerAPI).IStorageMgr.Remove(ctx, storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sid),
			},
		})
		require.NoError(t, err)
	}

	waitUntil = di.Close + di.WPoStChallengeWindow
	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(sectors-1)))
}
