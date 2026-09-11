package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

func TestWindowPostNoPreChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs(),
		kit.ConstructorOpts(
			node.Override(new(*wdpost.WindowPoStScheduler), modules.WindowPostScheduler(
				config.DefaultStorageMiner().Fees,
				config.ProvingConfig{
					DisableWDPoStPreChecks: true,
				},
			))))
	ens.InterconnectAll().BeginMining(2 * time.Millisecond)

	nSectors := 10

	miner.PledgeSectors(ctx, nSectors, 0, nil)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)
	di := client.CurrentProvingDeadline(ctx, maddr)

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors+kit.DefaultPresealsPerBootstrapMiner)))

	t.Log("Drop some sectors")

	// Drop 2 sectors from deadline 2 partition 0 (full partition / deadline)
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, 2, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the partition
		err = secs.ForEach(func(sid uint64) error {
			return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkCorrupted(storiface.SectorRef{
				ID: abi.SectorID{
					Miner:  abi.ActorID(mid),
					Number: abi.SectorNumber(sid),
				},
			}, true)
		})
		require.NoError(t, err)
	}

	var s storiface.SectorRef

	// Drop 1 sectors from deadline 3 partition 0
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, 3, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the sector
		sn, err := secs.First()
		require.NoError(t, err)

		all, err := secs.All(2)
		require.NoError(t, err)
		t.Log("the sectors", all)

		s = storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sn),
			},
		}

		err = miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(s, true)
		require.NoError(t, err)
	}

	di = client.CurrentProvingDeadline(ctx, maddr)

	t.Log("Go through another PP, wait for sectors to become faulty")
	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors := p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-3, int(sectors)) // -3 just removed sectors

	t.Log("Recover one sector")

	err = miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(s, false)
	require.NoError(t, err)

	di = client.CurrentProvingDeadline(ctx, maddr)

	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-2, int(sectors)) // -2 not recovered sectors

	// pledge a sector after recovery

	miner.PledgeSectors(ctx, 1, nSectors, nil)

	{
		// Wait until proven.
		di = client.CurrentProvingDeadline(ctx, maddr)

		waitUntil := di.Open + di.WPoStProvingPeriod
		t.Logf("End for head.Height > %d\n", waitUntil)

		ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
		t.Logf("Now head.Height = %d", ts.Height())
	}

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-2+1, int(sectors)) // -2 not recovered sectors + 1 just pledged
}

func TestWindowPostMaxSectorsRecoveryConfig(t *testing.T) {
	oldVal := wdpost.RecoveringSectorLimit
	defer func() {
		wdpost.RecoveringSectorLimit = oldVal
	}()
	wdpost.RecoveringSectorLimit = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs())
	ens.InterconnectAll().BeginMining(2 * time.Millisecond)

	nSectors := 10

	miner.PledgeSectors(ctx, nSectors, 0, nil)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)
	di := client.CurrentProvingDeadline(ctx, maddr)

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors+kit.DefaultPresealsPerBootstrapMiner)))

	t.Log("Drop some sectors")

	// Drop 2 sectors from deadline 2 partition 0 (full partition / deadline)
	parts, err := client.StateMinerPartitions(ctx, maddr, 2, types.EmptyTSK)
	require.NoError(t, err)
	require.Greater(t, len(parts), 0)

	secs := parts[0].AllSectors
	n, err := secs.Count()
	require.NoError(t, err)
	require.Equal(t, uint64(2), n)

	// Drop the partition
	err = secs.ForEach(func(sid uint64) error {
		return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sid),
			},
		}, true)
	})
	require.NoError(t, err)

	di = client.CurrentProvingDeadline(ctx, maddr)

	t.Log("Go through another PP, wait for sectors to become faulty")
	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors := p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-2, int(sectors)) // -2 just removed sectors

	t.Log("Make the sectors recoverable")

	err = secs.ForEach(func(sid uint64) error {
		return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sid),
			},
		}, false)
	})
	require.NoError(t, err)

	di = client.CurrentProvingDeadline(ctx, maddr)

	waitUntil = di.Open + di.WPoStProvingPeriod + 200
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-1, int(sectors)) // -1 not recovered sector
}

func TestWindowPostManualSectorsRecovery(t *testing.T) {
	oldVal := wdpost.RecoveringSectorLimit
	defer func() {
		wdpost.RecoveringSectorLimit = oldVal
	}()
	wdpost.RecoveringSectorLimit = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs())
	ens.InterconnectAll().BeginMining(2 * time.Millisecond)

	nSectors := 10

	miner.PledgeSectors(ctx, nSectors, 0, nil)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)
	di := client.CurrentProvingDeadline(ctx, maddr)

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors+kit.DefaultPresealsPerBootstrapMiner)))

	failed, err := client.StateMinerFaults(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	failedCount, err := failed.Count()
	require.NoError(t, err)

	require.Equal(t, failedCount, uint64(0))

	t.Log("Drop some sectors")

	// Drop 2 sectors from deadline 2 partition 0 (full partition / deadline)
	parts, err := client.StateMinerPartitions(ctx, maddr, 2, types.EmptyTSK)
	require.NoError(t, err)
	require.Greater(t, len(parts), 0)

	secs := parts[0].AllSectors
	n, err := secs.Count()
	require.NoError(t, err)
	require.Equal(t, uint64(2), n)

	var failedSectors []abi.SectorNumber

	// Drop the partition
	err = secs.ForEach(func(sid uint64) error {
		failedSectors = append(failedSectors, abi.SectorNumber(sid))
		return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sid),
			},
		}, true)
	})
	require.NoError(t, err)

	di = client.CurrentProvingDeadline(ctx, maddr)

	t.Log("Go through another PP, wait for sectors to become faulty")
	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	failed, err = client.StateMinerFaults(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	failedCount, err = failed.Count()
	require.NoError(t, err)

	require.Equal(t, failedCount, uint64(2))

	recovered, err := client.StateMinerRecoveries(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	recoveredCount, err := recovered.Count()
	require.NoError(t, err)

	require.Equal(t, recoveredCount, uint64(0))

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	t.Log("Make the sectors recoverable")

	err = secs.ForEach(func(sid uint64) error {
		return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sid),
			},
		}, false)
	})
	require.NoError(t, err)

	// Try to manually recover the sector
	t.Log("Send recovery message")
	_, err = miner.RecoverFault(ctx, failedSectors)
	require.NoError(t, err)

	currentHeight, err := client.ChainHead(ctx)
	require.NoError(t, err)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(currentHeight.Height()+abi.ChainEpoch(10)))
	t.Logf("Now head.Height = %d", ts.Height())

	failed, err = client.StateMinerFaults(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	failedCount, err = failed.Count()
	require.NoError(t, err)

	require.Equal(t, failedCount, uint64(2))

	recovered, err = client.StateMinerRecoveries(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	recoveredCount, err = recovered.Count()
	require.NoError(t, err)

	require.Equal(t, recoveredCount, uint64(2))

	di = client.CurrentProvingDeadline(ctx, maddr)

	t.Log("Go through another PP, wait for sectors to become faulty")
	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	failed, err = client.StateMinerFaults(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	failedCount, err = failed.Count()
	require.NoError(t, err)

	require.Equal(t, failedCount, uint64(0))

	recovered, err = client.StateMinerRecoveries(ctx, maddr, types.TipSetKey{})
	require.NoError(t, err)
	recoveredCount, err = recovered.Count()
	require.NoError(t, err)

	require.Equal(t, recoveredCount, uint64(0))
}

// TestDeadlineNotAfter is a unit test for the clamp behind
// kit.CurrentProvingDeadline. It lives here rather than in itests/kit because
// cmd/ci treats every _test.go under itests/ as an integration test group.
func TestDeadlineNotAfter(t *testing.T) {
	const (
		period      = abi.ChainEpoch(2880)
		window      = abi.ChainEpoch(60)
		periodStart = abi.ChainEpoch(658)
	)
	mk := func(index uint64, cur abi.ChainEpoch) *dline.Info {
		return dline.NewInfo(periodStart, index, cur, 48, period, window, 20, 70)
	}

	t.Run("deadline containing height is unchanged", func(t *testing.T) {
		di := mk(37, periodStart+37*window+5)
		require.Same(t, di, kit.DeadlineNotAfter(di, di.CurrentEpoch))
	})

	t.Run("deadline before height is unchanged", func(t *testing.T) {
		di := mk(37, periodStart+38*window)
		require.Same(t, di, kit.DeadlineNotAfter(di, di.CurrentEpoch))
	})

	// What StateMinerProvingDeadline returns at a deadline close when the
	// preceding epochs were null rounds: the recorded deadline has elapsed, so
	// NextNotElapsed moves it to the next proving period with the same index.
	for _, tc := range []struct {
		name  string
		index uint64
		nulls abi.ChainEpoch
	}{
		{"one null round", 37, 1},
		{"two null rounds", 37, 2},
		{"one null round at the last deadline of the period", 47, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorded := mk(tc.index, periodStart+abi.ChainEpoch(tc.index)*window)
			height := recorded.Close + tc.nulls - 1
			jumped := dline.NewInfo(periodStart, tc.index, height, 48, period, window, 20, 70).NextNotElapsed()
			require.Equal(t, periodStart+period, jumped.PeriodStart)
			require.Greater(t, jumped.Open, height)

			got := kit.DeadlineNotAfter(jumped, height)
			require.Equal(t, periodStart, got.PeriodStart)
			require.Equal(t, tc.index, got.Index)
			require.Equal(t, recorded.Open, got.Open)
			require.Equal(t, recorded.Close, got.Close)
			require.Equal(t, height, got.CurrentEpoch)
		})
	}

	t.Run("rewinds multiple periods", func(t *testing.T) {
		height := periodStart + 37*window
		di := dline.NewInfo(periodStart+3*period, 37, height, 48, period, window, 20, 70)
		got := kit.DeadlineNotAfter(di, height)
		require.Equal(t, periodStart, got.PeriodStart)
		require.Equal(t, height, got.Open)
	})
}
