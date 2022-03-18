package itests

import (
	"context"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"golang.org/x/xerrors"
	"sync/atomic"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage"
	storage2 "github.com/filecoin-project/specs-storage/storage"
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

	bm.MineBlocks(ctx, 2*time.Millisecond)

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

		err = miner.BaseAPI.(*impl.StorageMinerAPI).IStorageMgr.Remove(ctx, storage2.SectorRef{
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

type badWorkerStorage struct {
	stores.Store

	badsector uint64
}

func (bs *badWorkerStorage) GenerateSingleVanillaProof(ctx context.Context, minerID abi.ActorID, si storiface.PostSectorChallenge, ppt abi.RegisteredPoStProof) ([]byte, error) {
	if atomic.LoadUint64(&bs.badsector) == uint64(si.SectorNumber) {
		return nil, xerrors.New("no proof for you")
	}
	return bs.Store.GenerateSingleVanillaProof(ctx, minerID, si, ppt)
}

func TestWindowPostWorkerSkipBadSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	var badStore *badWorkerStorage

	client, miner, _, ens := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}),
		kit.WithWorkerStorage(func(store stores.Store) stores.Store {
			badStore = &badWorkerStorage{
				Store:     store,
				badsector: 10000,
			}
			return badStore
		}))

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	bm := ens.InterconnectAll().BeginMining(2 * time.Millisecond)[0]

	di = di.NextNotElapsed()

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStChallengeWindow*2 + storage.SubmitConfidence
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

	bm.MineBlocks(ctx, 2*time.Millisecond)

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

		atomic.StoreUint64(&badStore.badsector, sid)
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
