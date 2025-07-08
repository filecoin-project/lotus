package itests

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	miner11 "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

func TestWorkerPledge(t *testing.T) {
	ctx := context.Background()
	_, miner, worker, ens := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithNoLocalSealing(true),
		kit.WithSealWorkerTasks) // no mock proofs

	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	miner.PledgeSectors(ctx, 1, 0, nil)
}

func TestWorkerPledgeSpread(t *testing.T) {
	ctx := context.Background()
	_, miner, worker, ens := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(),
		kit.WithSealWorkerTasks,
		kit.WithAssigner("spread"),
	) // no mock proofs

	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	miner.PledgeSectors(ctx, 1, 0, nil)
}

func TestWorkerPledgeLocalFin(t *testing.T) {
	ctx := context.Background()
	_, miner, worker, ens := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(),
		kit.WithSealWorkerTasks,
		kit.WithDisallowRemoteFinalize(true),
	) // no mock proofs

	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	miner.PledgeSectors(ctx, 1, 0, nil)
}

func TestWorkerDataCid(t *testing.T) {
	ctx := context.Background()
	_, miner, worker, _ := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithNoLocalSealing(true),
		kit.WithSealWorkerTasks) // no mock proofs

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	pi, err := miner.ComputeDataCid(ctx, 1016, strings.NewReader(strings.Repeat("a", 1016)))
	require.NoError(t, err)
	require.Equal(t, abi.PaddedPieceSize(1024), pi.Size)
	require.Equal(t, "baga6ea4seaqlhznlutptgfwhffupyer6txswamerq5fc2jlwf2lys2mm5jtiaeq", pi.PieceCID.String())

	bigPiece := abi.PaddedPieceSize(16 << 20).Unpadded()
	pi, err = miner.ComputeDataCid(ctx, bigPiece, strings.NewReader(strings.Repeat("a", int(bigPiece))))
	require.NoError(t, err)
	require.Equal(t, bigPiece.Padded(), pi.Size)
	require.Equal(t, "baga6ea4seaqmhoxl2ybw5m2wyd3pt3h4zmp7j52yumzu2rar26twns3uocq7yfa", pi.PieceCID.String())

	nonFullPiece := abi.PaddedPieceSize(10 << 20).Unpadded()
	pi, err = miner.ComputeDataCid(ctx, bigPiece, strings.NewReader(strings.Repeat("a", int(nonFullPiece))))
	require.NoError(t, err)
	require.Equal(t, bigPiece.Padded(), pi.Size)
	require.Equal(t, "baga6ea4seaqbxib4pdxs5cqdn3fmtj4rcxk6rx6ztiqmrx7fcpo3ymuxbp2rodi", pi.PieceCID.String())
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

	bm := ens.InterconnectAll().BeginMiningMustPost(2 * time.Millisecond)[0]

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

type badWorkerStorage struct {
	paths.Store

	t *testing.T

	badsector   *uint64
	notBadCount int
}

func (bs *badWorkerStorage) GenerateSingleVanillaProof(ctx context.Context, minerID abi.ActorID, si storiface.PostSectorChallenge, ppt abi.RegisteredPoStProof) ([]byte, error) {
	if atomic.LoadUint64(bs.badsector) == uint64(si.SectorNumber) {
		bs.notBadCount--
		bs.t.Logf("Generating proof for sector %d maybe bad nbc=%d", si.SectorNumber, bs.notBadCount)
		if bs.notBadCount < 0 {
			return nil, xerrors.New("no proof for you")
		}
	}
	bs.t.Logf("Generating proof for sector %d", si.SectorNumber)
	return bs.Store.GenerateSingleVanillaProof(ctx, minerID, si, ppt)
}

func TestWindowPostWorkerSkipBadSector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	var badsector uint64 = 100000

	client, miner, _, ens := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}),
		kit.WithWorkerStorage(func(store paths.Store) paths.Store {
			return &badWorkerStorage{
				Store:     store,
				badsector: &badsector,
				t:         t,
			}
		}),
		kit.ConstructorOpts(node.ApplyIf(node.IsType(repo.StorageMiner),
			node.Override(new(paths.Store), func(store *paths.Remote) paths.Store {
				return &badWorkerStorage{
					Store:       store,
					badsector:   &badsector,
					t:           t,
					notBadCount: 1,
				}
			}))))

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	bm := ens.InterconnectAll().BeginMiningMustPost(2 * time.Millisecond)[0]

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

		atomic.StoreUint64(&badsector, sid)
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

func TestWindowPostWorkerManualPoSt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	client, miner, _, _ := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}))

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	di = di.NextNotElapsed()

	tryDl := func(dl uint64) {
		p, err := miner.ComputeWindowPoSt(ctx, dl, types.EmptyTSK)
		require.NoError(t, err)
		require.Len(t, p, 1)
		require.Equal(t, dl, p[0].Deadline)
	}
	tryDl(0)
	tryDl(40)
	tryDl(di.Index + 4)
}

func TestWindowPostWorkerDisconnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sectors := 2 * 48 * 2

	_, miner, badWorker, ens := kit.EnsembleWorker(t,
		kit.PresealSectors(sectors), // 2 sectors per partition, 2 partitions in all 48 deadlines
		kit.LatestActorsAt(-1),
		kit.ThroughRPC(),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}))

	var goodWorker kit.TestWorker
	ens.Worker(miner, &goodWorker, kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt}), kit.ThroughRPC()).Start()

	// wait for all workers
	require.Eventually(t, func() bool {
		w, err := miner.WorkerStats(ctx)
		require.NoError(t, err)
		return len(w) == 3 // 2 post + 1 miner-builtin
	}, 10*time.Second, 100*time.Millisecond)

	tryDl := func(dl uint64) {
		p, err := miner.ComputeWindowPoSt(ctx, dl, types.EmptyTSK)
		require.NoError(t, err)
		require.Len(t, p, 1)
		require.Equal(t, dl, p[0].Deadline)
	}
	tryDl(0) // this will run on the not-yet-bad badWorker

	err := badWorker.Stop(ctx)
	require.NoError(t, err)

	tryDl(10) // will fail on the badWorker, then should retry on the goodWorker

	time.Sleep(15 * time.Second)

	tryDl(40) // after HeartbeatInterval, the badWorker should be marked as disabled
}

func TestSchedulerRemoveRequest(t *testing.T) {
	ctx := context.Background()
	_, miner, worker, _ := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithNoLocalSealing(true),
		kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTAddPiece, sealtasks.TTPreCommit1})) // no mock proofs

	//ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Enabled(ctx)
	require.NoError(t, err)
	require.True(t, e)

	tocheck := miner.StartPledge(ctx, 1, 0, nil)
	var sn abi.SectorNumber
	for n := range tocheck {
		sn = n
	}
	// Keep checking till sector state is PC2, the request should get stuck as worker cannot process PC2
	for {
		st, err := miner.SectorsStatus(ctx, sn, false)
		require.NoError(t, err)
		if st.State == api.SectorState(sealing.PreCommit2) {
			break
		}
		time.Sleep(time.Second)
	}

	// Dump current scheduler info
	b := miner.SchedInfo(ctx)

	// cast scheduler info and get the request UUID. Call the SealingRemoveRequest()
	require.Len(t, b.SchedInfo.Requests, 1)
	require.Equal(t, "seal/v0/precommit/2", b.SchedInfo.Requests[0].TaskType)

	err = miner.SealingRemoveRequest(ctx, b.SchedInfo.Requests[0].SchedId)
	require.NoError(t, err)

	// Dump the scheduler again and compare the UUID if a request is present
	// If no request present then pass the test
	a := miner.SchedInfo(ctx)

	require.Len(t, a.SchedInfo.Requests, 0)
}

func TestWorkerName(t *testing.T) {
	name := "thisstringisprobablynotahostnameihope"

	ctx := context.Background()
	_, miner, worker, ens := kit.EnsembleWorker(t, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.WithWorkerName(name))

	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	e, err := worker.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, name, e.Hostname)

	ws, err := miner.WorkerStats(ctx)
	require.NoError(t, err)

	var found bool
	for _, stats := range ws {
		if stats.Info.Hostname == name {
			found = true
		}
	}

	require.True(t, found)
}

// Tests that V1_1 proofs on post worker
func TestWindowPostV1P1NV20Worker(t *testing.T) {
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blocktime := 2 * time.Millisecond

	client, miner, _, ens := kit.EnsembleWorker(t,
		kit.GenesisNetworkVersion(network.Version20),
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

	ens.InterconnectAll().BeginMiningMustPost(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	mi, err := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	wact, err := client.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
	require.NoError(t, err)
	en := wact.Nonce

	// wait for a new message to be sent from worker address, it will be a PoSt

waitForProof:
	for {
		wact, err := client.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
		require.NoError(t, err)
		if wact.Nonce > en {
			break waitForProof
		}

		build.Clock.Sleep(blocktime)
	}

	slm, err := client.StateListMessages(ctx, &api.MessageMatch{To: maddr}, types.EmptyTSK, 0)
	require.NoError(t, err)

	pmr, err := client.StateSearchMsg(ctx, types.EmptyTSK, slm[0], -1, false)
	require.NoError(t, err)

	nv, err := client.StateNetworkVersion(ctx, pmr.TipSet)
	require.NoError(t, err)
	require.Equal(t, network.Version20, nv)

	require.True(t, pmr.Receipt.ExitCode.IsSuccess())

	slmsg, err := client.ChainGetMessage(ctx, slm[0])
	require.NoError(t, err)

	var params miner11.SubmitWindowedPoStParams
	require.NoError(t, params.UnmarshalCBOR(bytes.NewBuffer(slmsg.Params)))
	require.Equal(t, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1_1, params.Proofs[0].PoStProof)
}
