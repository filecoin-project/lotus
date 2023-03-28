package itests

import (
	"context"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

func TestWindowPostNoMinerStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("storageminer", "INFO")

	sealSectors := 2
	presealSectors := 2*48*2 - sealSectors

	sectors := presealSectors + sealSectors

	var (
		client          kit.TestFullNode
		miner           kit.TestMiner
		wiw, wdw, sealw kit.TestWorker
	)
	ens := kit.NewEnsemble(t, kit.LatestActorsAt(-1)).
		FullNode(&client, kit.ThroughRPC()).
		Miner(&miner, &client, kit.WithAllSubsystems(), kit.ThroughRPC(), kit.PresealSectors(presealSectors), kit.NoStorage()).
		Worker(&miner, &wiw, kit.ThroughRPC(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWinningPoSt})).
		Worker(&miner, &wdw, kit.ThroughRPC(), kit.WithTaskTypes([]sealtasks.TaskType{sealtasks.TTGenerateWindowPoSt})).
		Worker(&miner, &sealw, kit.ThroughRPC(), kit.WithSealWorkerTasks).
		Start()

	ens.InterconnectAll().BeginMiningMustPost(10 * time.Millisecond)

	miner.PledgeSectors(ctx, sealSectors, 0, nil)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	di = di.NextNotElapsed()

	// wait for new sectors to become active
	waitUntil := di.Close + di.WPoStChallengeWindow*2 + di.WPoStProvingPeriod
	t.Logf("Wait Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(sectors)))
}
